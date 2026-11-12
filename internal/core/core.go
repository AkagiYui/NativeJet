package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"nativejet/internal/kernel"
	"nativejet/internal/logging"
	"nativejet/internal/model"
	"nativejet/internal/platform"
)

const logMessageBudget = 4 << 20

type Ring struct {
	mu    sync.Mutex
	seq   uint64
	items []model.Log
	bytes int
}

func (r *Ring) Add(level, source, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	if len(message) > 32<<10 {
		message = message[:32<<10] + "…"
	}
	r.items = append(r.items, model.Log{Sequence: r.seq, Time: time.Now(), Level: logging.Level(level), Source: source, Message: message})
	r.bytes += len(message)
	for len(r.items) > model.LogCacheLimit || r.bytes > logMessageBudget {
		r.bytes -= len(r.items[0].Message)
		r.items[0] = model.Log{}
		r.items = r.items[1:]
	}
}
func (r *Ring) Cursor() uint64 { r.mu.Lock(); defer r.mu.Unlock(); return r.seq }
func (r *Ring) Since(after uint64) []model.Log {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []model.Log{}
	for _, l := range r.items {
		if l.Sequence > after {
			out = append(out, l)
		}
	}
	return out
}

type Supervisor struct {
	mu         sync.Mutex
	record     *model.ProcessRecord
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	controller Controller
	done       chan struct{}
	traffic    model.Traffic
	trafficLog trafficHistory
	Logs       Ring
}

func (s *Supervisor) Record() *model.ProcessRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil {
		return nil
	}
	r := *s.record
	return &r
}
func (s *Supervisor) Running() bool {
	r := s.Record()
	if r == nil {
		return false
	}
	path, stamp, err := platform.ProcessIdentity(r.PID)
	return err == nil && platform.SamePath(path, r.Path) && stamp == r.StartTime
}
func (s *Supervisor) Restore(r *model.ProcessRecord, controller Controller) bool {
	if r == nil {
		return false
	}
	s.mu.Lock()
	copy := *r
	s.record = &copy
	s.controller = controller
	s.traffic = model.Traffic{}
	s.mu.Unlock()
	if !s.Running() {
		s.mu.Lock()
		s.record = nil
		s.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	if controller.address != "" && !r.ControllerOccupied {
		go s.followLogs(ctx, controller, r.PID)
		go s.followTraffic(ctx, controller, &copy)
	}
	s.Logs.Add("info", "helper", "已恢复对现有内核进程的管理")
	return true
}
func (s *Supervisor) Start(ctx context.Context, binary, expectedSHA256, home, config, revision string, controller Controller) error {
	if s.Running() {
		return errors.New("内核已经运行")
	}
	occupied := controller.occupied(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	verified, err := kernel.OpenVerified(binary, expectedSHA256)
	if err != nil {
		return err
	}
	defer verified.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	after := s.Logs.Cursor()
	cmd := exec.Command(binary, "-d", home, "-f", config)
	cmd.Dir = home
	platform.ConfigureCoreCommand(cmd, home)
	stdout, stderr := &processOutput{logs: &s.Logs}, &processOutput{logs: &s.Logs}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("内核启动失败: %w", err)
	}
	path, stamp, err := platform.ProcessIdentity(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		stdout.flush()
		stderr.flush()
		return logging.Failure("内核启动失败", err, s.Logs.Since(after))
	}
	r := &model.ProcessRecord{PID: cmd.Process.Pid, Path: path, StartTime: stamp, RevisionID: revision, ControllerOccupied: occupied}
	life, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.record = r
	s.controller = controller
	s.cmd = cmd
	s.cancel = cancel
	s.done = done
	s.traffic = model.Traffic{}
	s.mu.Unlock()
	go func() {
		err := cmd.Wait()
		stdout.flush()
		stderr.flush()
		defer close(done)
		cancel()
		s.mu.Lock()
		if s.record == r {
			s.record = nil
			s.cmd = nil
			s.traffic = model.Traffic{}
		}
		s.mu.Unlock()
		if err != nil {
			s.Logs.Add("error", "helper", "内核退出: "+err.Error())
		} else {
			s.Logs.Add("info", "helper", "内核已退出")
		}
	}()
	if controller.address != "" && !occupied {
		go s.followLogs(life, controller, r.PID)
		go s.followTraffic(life, controller, r)
	}
	// Capture an immediate process exit and its stderr during startup.
	grace := time.NewTimer(time.Second)
	defer grace.Stop()
	select {
	case <-ctx.Done():
		return errors.Join(ctx.Err(), s.Stop())
	case <-done:
		return logging.Failure("内核启动失败", errors.New("内核启动后退出；请查看启动输出"), s.Logs.Since(after))
	case <-grace.C:
		return nil
	}
}
func (s *Supervisor) Stop() error {
	r := s.Record()
	if r == nil {
		return nil
	}
	s.mu.Lock()
	done := s.done
	if s.cancel != nil {
		s.cancel()
	}
	s.traffic = model.Traffic{}
	s.mu.Unlock()
	path, stamp, err := platform.ProcessIdentity(r.PID)
	if err != nil {
		s.mu.Lock()
		s.record = nil
		s.mu.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-time.After(1500 * time.Millisecond):
			}
		}
		return nil
	}
	if !platform.SamePath(path, r.Path) || stamp != r.StartTime {
		return errors.New("内核进程身份已变化，拒绝向不相关进程发送信号")
	}
	process, err := os.FindProcess(r.PID)
	if err != nil {
		return err
	}
	if err = platform.InterruptCore(process, r.Path, r.StartTime); err != nil && !errors.Is(err, os.ErrProcessDone) {
		if !platform.ForceStopOnInterruptFailure {
			return err
		}
		s.Logs.Add("warning", "helper", "无法请求内核正常退出: "+err.Error())
	}
	until := time.Now().Add(6 * time.Second)
	for s.Running() && time.Now().Before(until) {
		time.Sleep(50 * time.Millisecond)
	}
	if s.Running() {
		path, stamp, _ := platform.ProcessIdentity(r.PID)
		if platform.SamePath(path, r.Path) && stamp == r.StartTime {
			if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("无法终止内核进程: %w", err)
			}
			s.Logs.Add("warning", "helper", "正常退出超时，已终止内核进程")
		}
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(1500 * time.Millisecond):
		}
	}
	s.mu.Lock()
	s.record = nil
	s.cmd = nil
	s.mu.Unlock()
	return nil
}

// os/exec waits for these writers before Wait returns, so an early exit cannot
// discard the final stderr line. Oversized lines are split within a fixed cap.
type processOutput struct {
	logs    *Ring
	pending string
}

func (w *processOutput) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		take := min(len(p), (32<<10)-len(w.pending))
		if index := bytes.IndexByte(p[:take], '\n'); index >= 0 {
			w.pending += string(p[:index])
			w.flush()
			p = p[index+1:]
		} else {
			w.pending += string(p[:take])
			p = p[take:]
			if len(w.pending) == 32<<10 {
				w.flush()
			}
		}
	}
	return n, nil
}
func (w *processOutput) flush() {
	if strings.TrimSpace(w.pending) != "" {
		level, _ := logging.Parse(w.pending)
		w.logs.Add(level, "process", w.pending)
	}
	w.pending = ""
}
func (s *Supervisor) followLogs(ctx context.Context, controller Controller, pid int) {
	for ctx.Err() == nil {
		resp, err := controller.request(ctx, pid, http.MethodGet, "/logs?level=debug", nil)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				sc := bufio.NewScanner(resp.Body)
				sc.Buffer(make([]byte, 4096), 128<<10)
				for sc.Scan() {
					var line struct {
						Type    string `json:"type"`
						Payload string `json:"payload"`
					}
					if json.Unmarshal(sc.Bytes(), &line) == nil && line.Payload != "" {
						s.Logs.Add(line.Type, "core", line.Payload)
					}
				}
			}
			resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
