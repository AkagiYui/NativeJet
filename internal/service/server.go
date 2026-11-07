package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"nativejet/internal/buildinfo"
	"nativejet/internal/ipc"
	"nativejet/internal/model"
	"nativejet/internal/platform"
)

type Server struct {
	Development                 bool
	Installed                   bool
	Socket, Data, Inbox, TeamID string
	PeerRequirement             string
	ApprovedKernels             map[string]bool
	stop                        context.CancelFunc
	recoveryError               string
	mu                          sync.Mutex
	startMu                     sync.Mutex
	managers                    map[int]*Manager
}
type authenticatedConn struct {
	net.Conn
	uid int
}
type authenticatedListener struct {
	net.Listener
	requirement string
	development bool
	installed   bool
}

func (l *authenticatedListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		u, ok := conn.(*net.UnixConn)
		if !ok {
			conn.Close()
			continue
		}
		raw, err := u.SyscallConn()
		if err != nil {
			conn.Close()
			continue
		}
		var uid int
		var auth error
		err = raw.Control(func(fd uintptr) { uid, auth = platform.Peer(fd, l.requirement) })
		if err != nil || auth != nil || (l.development && uid != os.Getuid()) || (!l.development && !l.installed && uid == 0) {
			conn.Close()
			continue
		}
		return &authenticatedConn{Conn: conn, uid: uid}, nil
	}
}

type uidKey struct{}

func (s *Server) manager(ctx context.Context, uid int) (*Manager, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.managers[uid]; m != nil {
		return m, nil
	}
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return nil, err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return nil, err
	}
	data := s.Data
	inbox := s.Inbox
	if data == "" {
		data = filepath.Join("/Library/Application Support/NativeJet/users", strconv.Itoa(uid))
	}
	if inbox == "" {
		inbox = filepath.Join(u.HomeDir, "Library/Application Support/NativeJet/kernels")
	}
	m, err := New(Options{Data: data, Inbox: inbox, UID: uid, GID: gid, ApprovedKernels: s.ApprovedKernels})
	if err != nil {
		return nil, err
	}
	if s.managers == nil {
		s.managers = map[int]*Manager{}
	}
	s.managers[uid] = m
	go m.Schedule(ctx)
	return m, nil
}
func (s *Server) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.stop = cancel
	requirement := s.PeerRequirement
	if !s.Development {
		if os.Geteuid() != 0 {
			return errors.New("生产 helper 必须由已授权的 LaunchDaemon 启动")
		}
		if s.Installed {
			if s.ApprovedKernels == nil {
				return errors.New("已安装服务缺少内核授权记录")
			}
			requirement = ""
		} else if requirement == "" {
			var err error
			requirement, err = platform.Requirement(buildinfo.AppID, s.TeamID)
			if err != nil {
				return err
			}
		}
	}
	if s.Socket == "" {
		return errors.New("未指定 IPC socket")
	}
	if _, err := os.Lstat(s.Socket); err == nil {
		conn, err := net.DialTimeout("unix", s.Socket, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return errors.New("后台服务已经运行")
		}
		if err = os.Remove(s.Socket); err != nil {
			return err
		}
	}
	listener, err := net.Listen("unix", s.Socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(s.Socket)
	mode := os.FileMode(0666)
	if s.Development {
		mode = 0600
	}
	if err = os.Chmod(s.Socket, mode); err != nil {
		return err
	}
	// Reconcile every persisted owner before accepting another user's start
	// request; an old core can outlive an unexpected helper termination.
	if !s.Development {
		entries, _ := os.ReadDir("/Library/Application Support/NativeJet/users")
		for _, entry := range entries {
			uid, err := strconv.Atoi(entry.Name())
			if err != nil || uid <= 0 || !entry.IsDir() {
				continue
			}
			if _, err := s.manager(ctx, uid); err != nil {
				s.recoveryError = fmt.Sprintf("无法恢复已有用户 %d 的后台状态: %v", uid, err)
			}
		}
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10, ConnContext: func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, uidKey{}, c.(*authenticatedConn).uid)
	}}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response ipc.Response
		if r.URL.Path != "/v1" || r.Method != http.MethodPost {
			http.Error(w, "not found", 404)
			return
		}
		var request ipc.Request
		r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			response.Error = "无效请求"
		} else if request.Version != model.ProtocolVersion {
			response.Error = "App 与后台服务版本不匹配，请更新后台服务"
		} else {
			uid, _ := r.Context().Value(uidKey{}).(int)
			m, err := s.manager(ctx, uid)
			if err != nil {
				response.Error = err.Error()
			} else {
				result, err := s.dispatch(r.Context(), uid, m, request)
				if err != nil {
					response.Error = err.Error()
				} else {
					response.Data, _ = json.Marshal(result)
				}
			}
		}
		_ = json.NewEncoder(w).Encode(response)
	})
	go func() { <-ctx.Done(); _ = server.Close() }()
	err = server.Serve(&authenticatedListener{Listener: listener, requirement: requirement, development: s.Development, installed: s.Installed})
	s.mu.Lock()
	managers := make([]*Manager, 0, len(s.managers))
	for _, m := range s.managers {
		managers = append(managers, m)
	}
	s.mu.Unlock()
	for _, m := range managers {
		m.Shutdown()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (s *Server) dispatch(ctx context.Context, uid int, m *Manager, r ipc.Request) (any, error) {
	var args struct {
		ID       string        `json:"id"`
		Name     *string       `json:"name"`
		URL      string        `json:"url"`
		Text     string        `json:"text"`
		Interval int           `json:"interval"`
		AssetID  int64         `json:"assetId"`
		After    uint64        `json:"after"`
		History  bool          `json:"history"`
		Profile  model.Profile `json:"profile"`
		Content  *string       `json:"content"`
		Global   *string       `json:"global"`
		GlobalID string        `json:"globalOverrideId"`
		Override *string       `json:"override"`
		State    *model.State  `json:"state"`
	}
	if len(r.Params) > 0 && string(r.Params) != "null" {
		if err := json.Unmarshal(r.Params, &args); err != nil {
			return nil, errors.New("无效操作参数")
		}
	}
	switch r.Method {
	case "service.shutdown":
		if !s.Development {
			return nil, errors.New("已安装服务只能通过界面授权卸载")
		}
		if err := m.Stop(); err != nil {
			return nil, err
		}
		time.AfterFunc(100*time.Millisecond, s.stop)
		return nil, nil
	case "profiles.import":
		if !s.Development || args.State == nil {
			return nil, errors.New("此模式不允许导入配置状态")
		}
		return nil, m.ImportProfiles(*args.State)
	case "snapshot":
		return m.Snapshot(), nil
	case "profile.save":
		return m.SaveProfile(args.Profile)
	case "profile.delete":
		return nil, m.DeleteProfile(args.ID)
	case "profile.subscribe":
		if args.Name == nil {
			return nil, errors.New("配置名称不能为空")
		}
		return m.Subscribe(ctx, *args.Name, args.URL, args.Interval)
	case "profile.update":
		return nil, m.UpdateSubscription(ctx, args.ID)
	case "override.save":
		return nil, m.SaveOverride(args.ID, args.Text)
	case "globalOverride.save":
		return nil, m.SaveGlobalOverride(args.ID, args.Name, args.Content)
	case "globalOverride.select":
		return nil, m.SelectGlobalOverride(args.ID)
	case "globalOverride.delete":
		return nil, m.DeleteGlobalOverride(args.ID)
	case "config.preview":
		return m.Preview(args.ID, args.GlobalID, args.Content, args.Global, args.Override)
	case "config.apply":
		return nil, m.Apply(ctx, args.ID)
	case "config.restore":
		return nil, m.Restore(ctx, args.ID)
	case "revision.yaml":
		return m.RevisionYAML(args.ID)
	case "kernel.scan":
		return m.Scan(ctx)
	case "kernel.select":
		return nil, m.SelectKernel(ctx, args.ID)
	case "kernel.releases":
		return m.Releases(ctx)
	case "kernel.download":
		return m.Download(ctx, args.AssetID)
	case "core.start", "core.restart":
		s.startMu.Lock()
		defer s.startMu.Unlock()
		if s.recoveryError != "" {
			return nil, errors.New(s.recoveryError)
		}
		s.mu.Lock()
		other := false
		for owner, manager := range s.managers {
			if owner != uid && manager.core.Running() {
				other = true
			}
		}
		s.mu.Unlock()
		if other {
			return nil, errors.New("另一位已登录用户正在使用系统内核，请先停止该会话")
		}
		if r.Method == "core.restart" {
			if err := m.Stop(); err != nil {
				return nil, err
			}
		}
		return nil, m.Start(ctx)
	case "core.stop":
		return nil, m.Stop()
	case "traffic.snapshot":
		return m.core.Traffic(args.History), nil
	case "logs":
		return m.Logs(args.After), nil
	case "connections":
		data, err := m.Core(ctx, http.MethodGet, "/connections", nil)
		if err != nil {
			return nil, err
		}
		var out model.Connections
		err = json.Unmarshal(data, &out)
		return out, err
	case "connections.close":
		if len(args.ID) > 200 {
			return nil, errors.New("无效连接 ID")
		}
		_, err := m.Core(ctx, http.MethodDelete, "/connections/"+url.PathEscape(args.ID), nil)
		return nil, err
	case "connections.closeAll":
		_, err := m.Core(ctx, http.MethodDelete, "/connections", nil)
		return nil, err
	case "proxies":
		data, err := m.Core(ctx, http.MethodGet, "/proxies", nil)
		if err != nil {
			return nil, err
		}
		var out struct {
			Proxies map[string]model.Proxy `json:"proxies"`
		}
		if err = json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		groups := []model.Proxy{}
		for name, p := range out.Proxies {
			if len(p.All) > 0 {
				p.Name = name
				groups = append(groups, p)
			}
		}
		sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
		return groups, nil
	case "proxies.select":
		data, err := m.Core(ctx, http.MethodGet, "/proxies/"+url.PathEscape(args.ID), nil)
		if err != nil {
			return nil, err
		}
		var proxy model.Proxy
		if err = json.Unmarshal(data, &proxy); err != nil {
			return nil, err
		}
		if proxy.Type != "Selector" || args.Name == nil || !slices.Contains(proxy.All, *args.Name) {
			return nil, errors.New("该策略组不支持选择此节点")
		}
		_, err = m.Core(ctx, http.MethodPut, "/proxies/"+url.PathEscape(args.ID), map[string]string{"name": *args.Name})
		return nil, err
	default:
		return nil, fmt.Errorf("未知操作 %q", r.Method)
	}
}
