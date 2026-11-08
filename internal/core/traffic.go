package core

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"nativejet/internal/model"
	"nativejet/internal/platform"
)

const trafficIdleTimeout = 4 * time.Second

func (s *Supervisor) Traffic(includeHistory bool) model.TrafficSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := model.TrafficSnapshot{Traffic: s.traffic}
	if includeHistory {
		snapshot.History = s.trafficLog.since(time.Now().Add(-model.TrafficHistoryWindow))
	}
	return snapshot
}

// Allow a little slack above Mihomo's one sample per second.
const trafficHistoryCapacity = int(model.TrafficHistoryWindow/time.Second) + 16

// trafficHistory is owned by the supervisor and guarded by its mutex.
type trafficHistory struct {
	points [trafficHistoryCapacity]model.TrafficPoint
	next   int
	count  int
}

func (h *trafficHistory) add(point model.TrafficPoint) {
	if h.count != 0 {
		lastIndex := (h.next + len(h.points) - 1) % len(h.points)
		last := h.points[lastIndex]
		point.BreakBefore = point.BreakBefore || point.At.Sub(last.At) > 1500*time.Millisecond
	}
	h.points[h.next] = point
	h.next = (h.next + 1) % len(h.points)
	h.count = min(h.count+1, len(h.points))
}

func (h *trafficHistory) since(cutoff time.Time) []model.TrafficPoint {
	first := (h.next + len(h.points) - h.count) % len(h.points)
	var points []model.TrafficPoint
	for i := range h.count {
		point := h.points[(first+i)%len(h.points)]
		if !point.At.Before(cutoff) {
			points = append(points, point)
		}
	}
	return points
}

// The process record doubles as the stream's identity. A late read from a
// replaced core must never publish a sample or clear the new core's sample.
func (s *Supervisor) followTraffic(ctx context.Context, controller Controller, record *model.ProcessRecord) {
	for ctx.Err() == nil {
		path, stamp, err := platform.ProcessIdentity(record.PID)
		if err != nil || !platform.SamePath(path, record.Path) || stamp != record.StartTime {
			return
		}
		s.readTraffic(ctx, controller, record)
		s.mu.Lock()
		if s.record == record {
			s.traffic = model.Traffic{}
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (s *Supervisor) readTraffic(ctx context.Context, controller Controller, record *model.ProcessRecord) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	idle := time.AfterFunc(trafficIdleTimeout, cancel)
	defer idle.Stop()

	resp, err := controller.request(ctx, record.PID, http.MethodGet, "/traffic", nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 512), 4<<10)
	first := true
	for sc.Scan() {
		var frame struct {
			Up        *int64 `json:"up"`
			Down      *int64 `json:"down"`
			UpTotal   *int64 `json:"upTotal"`
			DownTotal *int64 `json:"downTotal"`
		}
		if json.Unmarshal(sc.Bytes(), &frame) != nil ||
			frame.Up == nil || frame.Down == nil || *frame.Up < 0 || *frame.Down < 0 {
			continue
		}
		sample := model.Traffic{Available: true, Up: *frame.Up, Down: *frame.Down}
		if frame.UpTotal != nil && frame.DownTotal != nil && *frame.UpTotal >= 0 && *frame.DownTotal >= 0 {
			sample.TotalsAvailable = true
			sample.UpTotal, sample.DownTotal = *frame.UpTotal, *frame.DownTotal
		}
		s.mu.Lock()
		if ctx.Err() != nil || s.record != record {
			s.mu.Unlock()
			return
		}
		s.traffic = sample
		s.trafficLog.add(model.TrafficPoint{
			At: time.Now(), Up: *frame.Up, Down: *frame.Down, BreakBefore: first,
		})
		s.mu.Unlock()
		first = false
		if !idle.Reset(trafficIdleTimeout) {
			return
		}
	}
}
