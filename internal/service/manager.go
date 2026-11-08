package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"nativejet/internal/config"
	"nativejet/internal/core"
	"nativejet/internal/installation"
	"nativejet/internal/kernel"
	"nativejet/internal/model"
	"nativejet/internal/platform"
	"nativejet/internal/storage"
)

type Manager struct {
	mu              sync.RWMutex
	op              sync.Mutex
	state           model.State
	store           *storage.Store
	core            *core.Supervisor
	kernel          *kernel.Manager
	kernels         []model.Kernel
	uid, gid        int
	runtime         string
	lastError       string
	approvedKernels map[string]bool
}
type Options struct {
	Data, Inbox     string
	UID, GID        int
	ApprovedKernels map[string]bool
}

func New(o Options) (*Manager, error) {
	store, err := storage.New(o.Data)
	if err != nil {
		return nil, err
	}
	state, err := store.Load()
	if err != nil {
		return nil, err
	}
	if os.Geteuid() == 0 && o.UID != 0 {
		for _, command := range [][]string{{"/bin/mkdir", "-p", o.Inbox}, {"/bin/chmod", "700", o.Inbox}} {
			cmd := exec.Command(command[0], command[1:]...)
			cmd.SysProcAttr = platform.Credentials(o.UID, o.GID)
			if out, err := cmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("准备内核目录: %s: %w", out, err)
			}
		}
	} else if err = os.MkdirAll(o.Inbox, 0700); err != nil {
		return nil, err
	}
	runtimeDir := storage.RuntimeDirectory(o.Data)
	if _, err := storage.New(runtimeDir); err != nil {
		return nil, err
	}
	m := &Manager{state: state, store: store, uid: o.UID, gid: o.GID, runtime: runtimeDir, kernels: []model.Kernel{}, core: &core.Supervisor{}}
	m.approvedKernels = o.ApprovedKernels
	m.kernel = &kernel.Manager{Inbox: o.Inbox, Installed: store.Path("cores"), UID: o.UID, GID: o.GID}
	if state.Process != nil {
		var controller core.Controller
		if values, err := m.revisionValues(state.Process.RevisionID); err == nil {
			controller = core.ConfiguredController(values, m.runtime)
		}
		if !m.core.Restore(state.Process, controller) {
			state.Process = nil
			m.state = state
			_ = store.Save(state)
		}
	}
	m.kernels = append(m.kernels, state.Installed...)
	return m, nil
}
func (m *Manager) clone() model.State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, _ := json.Marshal(m.state)
	var s model.State
	_ = json.Unmarshal(b, &s)
	return s
}
func (m *Manager) commit(s model.State) error {
	if err := m.store.Save(s); err != nil {
		return err
	}
	m.mu.Lock()
	m.state = s
	m.mu.Unlock()
	return nil
}
func (m *Manager) error(err error) error {
	m.mu.Lock()
	if err == nil {
		m.lastError = ""
	} else {
		m.lastError = err.Error()
	}
	m.mu.Unlock()
	if err != nil {
		m.core.Logs.Add("error", "helper", err.Error())
	}
	return err
}
func find(s *model.State, id string) (*model.Profile, error) {
	for i := range s.Profiles {
		if s.Profiles[i].ID == id {
			return &s.Profiles[i], nil
		}
	}
	return nil, errors.New("找不到配置")
}
func (m *Manager) Snapshot() model.Snapshot {
	s := m.clone()
	m.mu.RLock()
	kernels := append([]model.Kernel{}, m.kernels...)
	lastError := m.lastError
	m.mu.RUnlock()
	status := model.Status{Protocol: model.ProtocolVersion, Privileged: os.Geteuid() == 0, Running: m.core.Running(), ActiveRevision: s.ActiveRevision, ActiveProfile: s.ActiveProfile, Error: lastError, KernelDirectory: m.kernel.Inbox}
	status.RuntimeDirectory = m.runtime
	status.GlobalOverrides = true
	status.GeoData = true
	status.KernelAuthorized = m.approvedKernels == nil || m.approvedKernels[s.SelectedKernel]
	for id := range m.approvedKernels {
		status.AuthorizedKernels = append(status.AuthorizedKernels, id)
	}
	slices.Sort(status.AuthorizedKernels)
	if r := m.core.Record(); r != nil {
		status.PID = r.PID
	}
	for _, k := range s.Installed {
		if k.ID == s.SelectedKernel {
			status.KernelVersion = k.Version
			status.KernelPath = k.Path
		}
	}
	if s.ActiveRevision != "" {
		status.ConfigPath = m.revisionPath(s.ActiveRevision)
		if data, err := os.ReadFile(status.ConfigPath); err == nil {
			if values, _, err := config.Parse(string(data)); err == nil {
				status.ActiveTUN = config.BoolAt(values, "tun.enable")
			}
		}
	}
	s.Process = m.core.Record()
	return model.Snapshot{State: s, Status: status, Kernels: kernels}
}
func (m *Manager) ImportProfiles(incoming model.State) error {
	m.op.Lock()
	defer m.op.Unlock()
	if m.core.Running() {
		return errors.New("请先停止内核再切换服务模式")
	}
	if err := installation.ValidateState(incoming); err != nil {
		return err
	}
	s := m.clone()
	for _, profile := range incoming.Profiles {
		index := slices.IndexFunc(s.Profiles, func(p model.Profile) bool { return p.ID == profile.ID })
		if index < 0 {
			s.Profiles = append(s.Profiles, profile)
		} else {
			s.Profiles[index] = profile
		}
	}
	s.GlobalOverrides = slices.Clone(incoming.GlobalOverrides)
	s.SelectedGlobalOverride = incoming.SelectedGlobalOverride
	if slices.ContainsFunc(s.Installed, func(k model.Kernel) bool { return k.ID == incoming.SelectedKernel }) {
		s.SelectedKernel = incoming.SelectedKernel
	}
	s.ActiveProfile = ""
	s.ActiveRevision = ""
	s.Process = nil
	return m.commit(s)
}
func (m *Manager) SaveProfile(p model.Profile) (model.Profile, error) {
	m.op.Lock()
	defer m.op.Unlock()
	s := m.clone()
	if p.Kind != "local" && p.Kind != "subscription" {
		return p, errors.New("无效配置类型")
	}
	if strings.TrimSpace(p.Name) == "" {
		return p, errors.New("配置名称不能为空")
	}
	if p.Kind == "local" {
		p.SubscriptionRaw = ""
	}
	if _, _, err := config.Parse(p.Content); err != nil {
		return p, err
	}
	if p.ID == "" {
		p.ID = storage.ID()
		p.UpdatedAt = time.Now()
		s.Profiles = append(s.Profiles, p)
	} else {
		old, err := find(&s, p.ID)
		if err != nil {
			return p, err
		}
		if old.Kind == "subscription" && (old.Content != p.Content || old.SubscriptionRaw != p.SubscriptionRaw) {
			return p, errors.New("订阅原文只读，请编辑覆盖配置或创建本地副本")
		}
		if old.Kind != p.Kind {
			return p, errors.New("配置类型不可直接改变")
		}
		p.UpdatedAt = time.Now()
		*old = p
	}
	if _, _, err := config.Parse(p.Override); err != nil {
		return p, err
	}
	return p, m.commit(s)
}
func (m *Manager) DeleteProfile(id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	s := m.clone()
	if id == s.ActiveProfile {
		return errors.New("当前运行配置不能删除，请先应用其他配置")
	}
	if _, err := find(&s, id); err != nil {
		return err
	}
	s.Profiles = slices.DeleteFunc(s.Profiles, func(p model.Profile) bool { return p.ID == id })
	return m.commit(s)
}
func (m *Manager) SaveOverride(id, text string) error {
	m.op.Lock()
	defer m.op.Unlock()
	if _, _, err := config.Parse(text); err != nil {
		return err
	}
	s := m.clone()
	if id == "" {
		s.GlobalOverrideByID(s.SelectedGlobalOverride).Content = text
	} else {
		p, err := find(&s, id)
		if err != nil {
			return err
		}
		p.Override = text
	}
	return m.commit(s)
}

func (m *Manager) SaveGlobalOverride(id string, name, content *string) error {
	m.op.Lock()
	defer m.op.Unlock()
	s := m.clone()
	preset := s.GlobalOverrideByID(id)
	if id == "" {
		s.GlobalOverrides = append(s.GlobalOverrides, model.GlobalOverride{ID: storage.ID()})
		preset = &s.GlobalOverrides[len(s.GlobalOverrides)-1]
		s.SelectedGlobalOverride = preset.ID
	} else if preset == nil {
		return errors.New("找不到全局覆盖")
	}
	if name != nil {
		preset.Name = strings.TrimSpace(*name)
	}
	if preset.Name == "" {
		return errors.New("全局覆盖名称不能为空")
	}
	if content != nil {
		if _, _, err := config.Parse(*content); err != nil {
			return err
		}
		preset.Content = *content
	}
	return m.commit(s)
}

func (m *Manager) SelectGlobalOverride(id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	s := m.clone()
	if s.GlobalOverrideByID(id) == nil {
		return errors.New("找不到全局覆盖")
	}
	s.SelectedGlobalOverride = id
	return m.commit(s)
}

func (m *Manager) DeleteGlobalOverride(id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	s := m.clone()
	i := slices.IndexFunc(s.GlobalOverrides, func(p model.GlobalOverride) bool { return p.ID == id })
	if i < 0 {
		return errors.New("找不到全局覆盖")
	}
	if len(s.GlobalOverrides) == 1 {
		return errors.New("请至少保留一份全局覆盖")
	}
	s.GlobalOverrides = slices.Delete(s.GlobalOverrides, i, i+1)
	if s.SelectedGlobalOverride == id {
		s.SelectedGlobalOverride = s.GlobalOverrides[min(i, len(s.GlobalOverrides)-1)].ID
	}
	return m.commit(s)
}

func (m *Manager) Subscribe(ctx context.Context, name, address string, interval int) (model.Profile, error) {
	if interval < 0 || interval > 8760 {
		return model.Profile{}, errors.New("更新间隔必须为 0–8760 小时")
	}
	p := model.Profile{ID: storage.ID(), Name: name, Kind: "subscription", URL: address, IntervalHours: interval, UpdatedAt: time.Now()}
	if strings.TrimSpace(name) == "" {
		return p, errors.New("配置名称不能为空")
	}
	if err := m.fetch(ctx, &p); err != nil {
		return p, err
	}
	m.op.Lock()
	defer m.op.Unlock()
	s := m.clone()
	s.Profiles = append(s.Profiles, p)
	return p, m.commit(s)
}
func (m *Manager) fetch(ctx context.Context, p *model.Profile) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return err
	}
	if req.URL.Host == "" || (req.URL.Scheme != "https" && req.URL.Scheme != "http") {
		return errors.New("订阅地址必须为 HTTP(S) URL")
	}
	req.Header.Set("User-Agent", "clash.meta")
	if p.ETag != "" {
		req.Header.Set("If-None-Match", p.ETag)
	}
	resp, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
	if err != nil {
		return errors.New("订阅下载失败，请检查地址或网络连接")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		p.UpdatedAt = time.Now()
		return nil
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("订阅服务器返回 %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxBytes+1))
	if err != nil {
		return err
	}
	if len(data) > config.MaxBytes {
		return errors.New("订阅超过 4 MB")
	}
	raw := string(data)
	content, err := config.SubscriptionYAML(raw)
	if err != nil {
		return err
	}
	p.Content = content
	p.SubscriptionRaw = ""
	if content != raw {
		p.SubscriptionRaw = raw
	}
	p.UpdatedAt = time.Now()
	p.ETag = resp.Header.Get("ETag")
	p.SubscriptionInfo = resp.Header.Get("Subscription-Userinfo")
	return nil
}
func (m *Manager) UpdateSubscription(ctx context.Context, id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	s := m.clone()
	p, err := find(&s, id)
	if err != nil {
		return err
	}
	if p.Kind != "subscription" {
		return errors.New("本地配置没有订阅更新")
	}
	if err = m.fetch(ctx, p); err != nil {
		return m.error(err)
	}
	if err = m.commit(s); err == nil {
		m.core.Logs.Add("info", "helper", "订阅原文已更新；运行配置保持不变")
	}
	return err
}
func (m *Manager) Schedule(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, p := range m.clone().Profiles {
				if p.Kind == "subscription" && p.IntervalHours > 0 && time.Since(p.UpdatedAt) >= time.Duration(p.IntervalHours)*time.Hour {
					update, cancel := context.WithTimeout(ctx, 50*time.Second)
					_ = m.UpdateSubscription(update, p.ID)
					cancel()
				}
			}
		}
	}
}

const convertedSubscriptionSource = "订阅节点（格式转换）"

func (m *Manager) compile(s model.State, id string, content, global, override *string) (model.Preview, map[string]any, error) {
	p, err := find(&s, id)
	if err != nil {
		return model.Preview{}, nil, err
	}
	source := "本地原文"
	if p.Kind == "subscription" {
		source = "订阅原文"
		if p.SubscriptionRaw != "" {
			source = convertedSubscriptionSource
		}
	}
	raw := p.Content
	if p.Kind == "local" && content != nil {
		raw = *content
	}
	preset := s.GlobalOverrideByID(s.SelectedGlobalOverride)
	if preset == nil {
		return model.Preview{}, nil, errors.New("找不到全局覆盖")
	}
	g := preset.Content
	if global != nil {
		g = *global
	}
	o := p.Override
	if override != nil {
		o = *override
	}
	return config.Compile(config.Layer{Name: source, Content: raw}, config.Layer{Name: "全局覆盖 · " + preset.Name, Content: g}, config.Layer{Name: "当前配置覆盖", Content: o})
}
func (m *Manager) Preview(id, globalID string, content, global, override *string) (model.Preview, error) {
	s := m.clone()
	if globalID != "" {
		s.SelectedGlobalOverride = globalID
	}
	p, _, err := m.compile(s, id, content, global, override)
	if err != nil {
		return p, err
	}
	p.YAML = config.Redact(p.YAML)
	for i := range p.Fields {
		f := &p.Fields[i]
		f.Value = config.RedactValue(f.Path, f.Value)
		for j := range f.Trace {
			f.Trace[j].Value = config.RedactValue(f.Path, f.Trace[j].Value)
			if f.Trace[j].Layer == convertedSubscriptionSource {
				// The generated YAML line does not identify a line in the raw subscription.
				f.Trace[j].Line = 0
			}
		}
	}
	return p, nil
}
func (m *Manager) Scan(ctx context.Context) ([]model.Kernel, error) {
	items, err := m.kernel.Scan(ctx)
	if err != nil {
		return nil, err
	}
	s := m.clone()
	for _, installed := range s.Installed {
		if !slices.ContainsFunc(items, func(k model.Kernel) bool { return k.ID == installed.ID }) {
			items = append(items, installed)
		}
	}
	m.mu.Lock()
	m.kernels = items
	m.mu.Unlock()
	return items, nil
}
func (m *Manager) SelectKernel(ctx context.Context, id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	if m.core.Running() {
		return errors.New("请先停止内核，再切换版本")
	}
	s := m.clone()
	if slices.ContainsFunc(s.Installed, func(k model.Kernel) bool { return k.ID == id }) {
		s.SelectedKernel = id
		return m.commit(s)
	}
	k, err := m.kernel.Install(ctx, id)
	if err != nil {
		return err
	}
	s.Installed = append(s.Installed, k)
	s.SelectedKernel = id
	return m.commit(s)
}
func (m *Manager) selected(s model.State) (model.Kernel, error) {
	for _, k := range s.Installed {
		if k.ID == s.SelectedKernel {
			if m.approvedKernels != nil && !m.approvedKernels[k.ID] {
				return k, errors.New("所选内核需要管理员授权，请点击“授权所选内核”")
			}
			hash, err := kernel.Hash(k.Path)
			if err != nil {
				return k, err
			}
			if hash != k.ID {
				return k, errors.New("受管理的内核文件身份发生变化")
			}
			return k, nil
		}
	}
	return model.Kernel{}, errors.New("请先扫描并选择内核")
}
func (m *Manager) revisionPath(id string) string {
	return filepath.Join(m.runtime, "revisions", id+".yaml")
}
func (m *Manager) revisionValues(id string) (map[string]any, error) {
	data, err := os.ReadFile(m.revisionPath(id))
	if err != nil {
		return nil, err
	}
	values, _, err := config.Parse(string(data))
	return values, err
}
func (m *Manager) Apply(ctx context.Context, id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	return m.error(m.apply(ctx, m.clone(), id))
}
func (m *Manager) apply(ctx context.Context, s model.State, id string) error {
	k, err := m.selected(s)
	if err != nil {
		return err
	}
	p, values, err := m.compile(s, id, nil, nil, nil)
	if err != nil {
		return err
	}
	preset := s.GlobalOverrideByID(s.SelectedGlobalOverride)
	r := model.Revision{ID: storage.ID(), ProfileID: id, KernelID: k.ID, Hash: p.Hash, CreatedAt: time.Now(),
		GlobalOverride: preset.Content, GlobalOverrideID: preset.ID, GlobalOverrideName: preset.Name}
	profile, _ := find(&s, id)
	r.Source = profile.Content
	r.SubscriptionRaw = profile.SubscriptionRaw
	r.ProfileOverride = profile.Override
	path := m.revisionPath(r.ID)
	if err = storage.Atomic(path, []byte(p.YAML), 0600); err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			_ = os.Remove(path)
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	old := m.clone()
	running := m.core.Running()
	if running {
		if err = m.requireTUNPrivilege(values); err != nil {
			return err
		}
		if err = m.core.Stop(); err != nil {
			return err
		}
		if err = m.startCore(ctx, k, r.ID, values); err != nil {
			return errors.Join(err, m.rollback(old))
		}
	}
	s.Revisions = append(s.Revisions, r)
	s.ActiveProfile = id
	s.ActiveRevision = r.ID
	s.Process = m.core.Record()
	if err = m.commit(s); err != nil {
		if running {
			return errors.Join(err, m.rollback(old))
		}
		return err
	}
	success = true
	m.core.Logs.Add("info", "helper", "已应用配置 "+r.ID[:8])
	return nil
}
func (m *Manager) rollback(s model.State) error {
	if s.ActiveRevision == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	values, err := m.revisionValues(s.ActiveRevision)
	if err != nil {
		_ = m.core.Stop()
		return fmt.Errorf("无法读取旧配置，已停止内核: %w", err)
	}
	if err := m.core.Stop(); err != nil {
		return err
	}
	k, err := m.selected(s)
	if err != nil {
		return err
	}
	err = m.startCore(ctx, k, s.ActiveRevision, values)
	if err == nil {
		s.Process = m.core.Record()
		err = m.commit(s)
	}
	if err != nil {
		_ = m.core.Stop()
		m.core.Logs.Add("error", "helper", "旧配置恢复失败: "+err.Error())
		return fmt.Errorf("旧配置恢复失败，已停止内核: %w", err)
	}
	m.core.Logs.Add("warning", "helper", "新配置应用失败，已用旧配置重启")
	return nil
}
func (m *Manager) requireTUNPrivilege(values map[string]any) error {
	if config.BoolAt(values, "tun.enable") && os.Geteuid() != 0 {
		return errors.New("TUN 需要已授权的 macOS 后台服务，请在“后台服务”中完成授权")
	}
	return nil
}
func (m *Manager) startCore(ctx context.Context, k model.Kernel, id string, values map[string]any) error {
	controller := core.ConfiguredController(values, m.runtime)
	return m.core.Start(ctx, k.Path, m.runtime, m.revisionPath(id), id, controller)
}
func (m *Manager) Start(ctx context.Context) error {
	m.op.Lock()
	defer m.op.Unlock()
	s := m.clone()
	if m.core.Running() {
		return nil
	}
	if s.ActiveProfile == "" {
		return errors.New("请先应用一份配置")
	}
	k, err := m.selected(s)
	if err != nil {
		return err
	}
	index := slices.IndexFunc(s.Revisions, func(r model.Revision) bool { return r.ID == s.ActiveRevision })
	if index < 0 {
		return errors.New("找不到已应用配置")
	}
	revision := s.Revisions[index]
	if revision.KernelID != k.ID {
		if err = m.apply(ctx, s, s.ActiveProfile); err != nil {
			return m.error(err)
		}
		s = m.clone()
		revision = s.Revisions[len(s.Revisions)-1]
	}
	preview, values, err := config.Compile(
		config.Layer{Name: "原文", Content: revision.Source},
		config.Layer{Name: "全局覆盖", Content: revision.GlobalOverride},
		config.Layer{Name: "当前配置覆盖", Content: revision.ProfileOverride},
	)
	if err != nil {
		return err
	}
	path := m.revisionPath(revision.ID)
	saved := false
	if preview.Hash != revision.Hash {
		revision.ID = storage.ID()
		revision.CreatedAt = time.Now()
		revision.Hash = preview.Hash
		path = m.revisionPath(revision.ID)
		if err = storage.Atomic(path, []byte(preview.YAML), 0600); err != nil {
			return err
		}
		defer func() {
			if !saved {
				_ = os.Remove(path)
			}
		}()
		s.ActiveRevision = revision.ID
		s.Revisions = append(s.Revisions, revision)
	} else if values, err = m.revisionValues(revision.ID); err != nil {
		return err
	}
	if err = m.requireTUNPrivilege(values); err != nil {
		return err
	}
	if err = m.startCore(ctx, k, s.ActiveRevision, values); err != nil {
		return m.error(err)
	}
	s.Process = m.core.Record()
	if err = m.commit(s); err != nil {
		_ = m.core.Stop()
		return err
	}
	saved = true
	return m.error(nil)
}
func (m *Manager) Stop() error {
	m.op.Lock()
	defer m.op.Unlock()
	if err := m.core.Stop(); err != nil {
		return m.error(err)
	}
	s := m.clone()
	s.Process = nil
	return m.commit(s)
}
func (m *Manager) Restore(ctx context.Context, id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	s := m.clone()
	for _, r := range s.Revisions {
		if r.ID == id {
			p, err := find(&s, r.ProfileID)
			if err != nil {
				return errors.New("历史版本所属配置已删除")
			}
			if p.Kind == "subscription" && (p.Content != r.Source || p.SubscriptionRaw != r.SubscriptionRaw) {
				p.ETag = ""
			}
			p.Content = r.Source
			p.SubscriptionRaw = r.SubscriptionRaw
			p.Override = r.ProfileOverride
			restoreGlobalOverride(&s, r)
			return m.error(m.apply(ctx, s, p.ID))
		}
	}
	return errors.New("找不到历史版本")
}

func restoreGlobalOverride(s *model.State, r model.Revision) {
	id := r.GlobalOverrideID
	if id == "" {
		id = s.SelectedGlobalOverride
	}
	if preset := s.GlobalOverrideByID(id); preset != nil && preset.Content == r.GlobalOverride {
		s.SelectedGlobalOverride = id
		return
	}
	name := "恢复的全局覆盖"
	if r.GlobalOverrideName != "" {
		name = r.GlobalOverrideName + " · 历史"
	}
	for _, preset := range s.GlobalOverrides {
		if preset.Name == name && preset.Content == r.GlobalOverride {
			s.SelectedGlobalOverride = preset.ID
			return
		}
	}
	preset := model.GlobalOverride{ID: storage.ID(), Name: name, Content: r.GlobalOverride}
	s.GlobalOverrides = append(s.GlobalOverrides, preset)
	s.SelectedGlobalOverride = preset.ID
}
func (m *Manager) Logs(after uint64) []model.Log { return m.core.Logs.Since(after) }
func (m *Manager) Core(ctx context.Context, method, path string, payload any) ([]byte, error) {
	if !m.core.Running() {
		return nil, errors.New("内核未运行")
	}
	return m.core.Call(ctx, method, path, payload)
}
func (m *Manager) Releases(ctx context.Context) ([]model.Release, error) {
	return m.kernel.Releases(ctx)
}
func (m *Manager) Download(ctx context.Context, id int64) (string, error) {
	m.op.Lock()
	defer m.op.Unlock()
	return m.kernel.Download(ctx, id)
}
func (m *Manager) Shutdown() { _ = m.Stop() }
func (m *Manager) RevisionYAML(id string) (string, error) {
	for _, r := range m.clone().Revisions {
		if r.ID == id {
			data, err := os.ReadFile(m.revisionPath(id))
			if err != nil {
				return "", err
			}
			return config.Redact(string(data)), nil
		}
	}
	return "", errors.New("找不到历史版本")
}
func ParseAfter(v string) uint64 { n, _ := strconv.ParseUint(v, 10, 64); return n }
