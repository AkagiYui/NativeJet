// Package app contains only presentation and helper-client code. It never
// starts a core process or talks to a core controller directly.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"nativejet/internal/buildinfo"
	"nativejet/internal/installation"
	"nativejet/internal/ipc"
	"nativejet/internal/model"
)

type App struct {
	win                                     *mygo.Window
	tray                                    *mygo.Tray
	trayCoreItem                            *mygo.MenuItem
	trayDisplay                             menuBarDisplay
	appearance                              appearancePreferences
	appearanceError                         string
	startup                                 startupPreferences
	startupError                            string
	loginItem                               loginItemSettings
	autoStartPending                        bool
	traffic                                 model.Traffic
	trafficHistory                          []model.TrafficPoint
	trafficClient                           *ipc.Client
	quitting                                bool
	importing                               *importForm
	globalNameWindow                        *mygo.Window
	pendingImports                          []subscriptionLink
	client                                  *ipc.Client
	connecting                              *ipc.Client
	developmentClient                       *ipc.Client
	registration                            *installation.Registration
	installing                              bool
	data, helperPath                        string
	snapshot                                model.Snapshot
	snapshotVersion                         uint64
	connected, busy, polling                bool
	operationCancel                         context.CancelFunc
	operationStarted                        time.Time
	message, problem                        string
	page, selected, loaded                  string
	loadedGlobal                            string
	tab, scope, format, field               int
	fields                                  ui.ListState
	source, global, override, final         *editor.State
	savedSource, savedGlobal, savedOverride string
	preview                                 model.Preview
	connections                             model.Connections
	connectionsError, proxiesError          error
	metricsTab                              int
	metrics                                 *metricsResult
	geoData                                 *geoResult
	nodeMetricsTable, ruleMetricsTable      metricTableState
	providerMetricsTable                    metricTableState
	connectionSort, kernelSort, logSort     ui.SortOrder
	connectionIndex                         int
	connectionRows                          ui.ListState
	kernelRows                              ui.ListState
	kernelSelection                         int
	logRows                                 ui.ListState
	logSearch                               string
	logSource                               string
	logSelection                            int
	logSelected, logDetailSequence          uint64
	logDetail                               *editor.State
	groups                                  []model.Proxy
	logs                                    []model.Log
	logAfter                                uint64
	logLevel, search                        string
	paused                                  bool
	helperState                             int
	helperError                             string
}

type Options struct {
	Data, Helper          string
	Development           bool
	PrivilegedDevelopment bool
	PreferInstalled       bool
}

func New(o Options) (*App, error) {
	// The system launches the app with its default arguments on login.
	manageLoginItem := o.PreferInstalled && o.Helper == ""
	if o.Data == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		o.Data = filepath.Join(base, "NativeJet")
	}
	socket := buildinfo.Socket
	if o.Development {
		if err := os.MkdirAll(o.Data, 0700); err != nil {
			return nil, err
		}
		socket = filepath.Join(o.Data, "helper.sock")
		if len(socket) > 100 {
			sum := sha256.Sum256([]byte(o.Data))
			socket = filepath.Join(os.TempDir(), "nativejet-ui-"+hex.EncodeToString(sum[:6])+".sock")
		}
	}
	if o.Helper == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		o.Helper = filepath.Join(filepath.Dir(filepath.Dir(exe)), "Library/LaunchServices/nativejet-helper")
	}
	a := &App{data: o.Data, helperPath: o.Helper, client: &ipc.Client{Socket: socket, Development: o.Development, TeamID: buildinfo.TeamID}, page: "配置", logLevel: "全部", logSource: "全部来源", logSelection: -1}
	a.loadAppearance()
	a.loadStartup()
	a.loginItem.available = manageLoginItem
	a.developmentClient = a.client
	if o.PreferInstalled {
		a.page = settingsPage
		if registration, err := installation.LoadRegistration(); err == nil {
			a.registration = &registration
			a.client = installedClient(registration)
			a.page = "配置"
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			a.helperError = err.Error()
		}
	}
	if o.PrivilegedDevelopment {
		requirement, err := signingRequirement(o.Helper)
		if err != nil {
			return nil, err
		}
		a.client.Requirement = requirement
	}
	a.fields.Selected = &a.field
	a.connectionRows.Selected = &a.connectionIndex
	a.connectionRows.Sort = &a.connectionSort
	a.kernelRows.Selected = &a.kernelSelection
	a.kernelRows.Sort = &a.kernelSort
	a.logRows.Selected = &a.logSelection
	a.logRows.Sort = &a.logSort
	a.logDetail = newEditor("日志完整内容", true)
	a.logDetail.Buffer.SetLanguage(nil)
	a.logDetail.LineNumbers = editor.LineNumbersOff
	a.logDetail.NoActiveLine = true
	a.source = newEditor("原始配置", false)
	a.global = newEditor("全局覆盖", false)
	a.override = newEditor("当前配置覆盖", false)
	a.final = newEditor("最终配置", true)
	return a, nil
}
func newEditor(label string, readonly bool) *editor.State {
	b := editor.NewBuffer("")
	b.SetLanguage(editor.LanguageFor("config.yaml"))
	return &editor.State{Buffer: b, ReadOnly: readonly, SoftWrap: true, Label: label, NoScrollPastEnd: true}
}
func (a *App) connect(parent context.Context) {
	client := a.client
	if a.connecting == client {
		return
	}
	a.connecting = client
	go a.connectUsing(parent, client)
}
func (a *App) connectUsing(parent context.Context, client *ipc.Client) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	snap, err := a.connectSnapshot(ctx, client)
	a.win.Update(func() {
		if a.connecting != client {
			return
		}
		a.connecting = nil
		if a.quitting || client != a.client || a.installing {
			return
		}
		defer a.updateTray()
		a.connected = err == nil
		if err != nil {
			a.problem = err.Error()
			a.message = "后台服务尚未连接"
			if a.registration != nil {
				a.page = settingsPage
				a.refreshLoginItem()
			}
		} else {
			a.accept(snap, false)
			a.message = "后台服务已连接"
			a.problem = snap.Status.Error
			a.maybeAutoStartCore()
		}
	})
}
func (a *App) connectSnapshot(ctx context.Context, client *ipc.Client) (model.Snapshot, error) {
	var snap model.Snapshot
	err := client.Call(ctx, "snapshot", nil, &snap)
	if err == nil || !client.Development {
		return snap, err
	}
	logfile, err := os.OpenFile(filepath.Join(a.data, "helper.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return snap, err
	}
	cmd := exec.Command(a.helperPath, "--development", "--socket", client.Socket, "--data-dir", filepath.Join(a.data, "development"), "--kernel-dir", filepath.Join(a.data, "kernels"))
	cmd.Stdout = logfile
	cmd.Stderr = logfile
	if err = cmd.Start(); err != nil {
		logfile.Close()
		return snap, fmt.Errorf("启动后台服务失败: %w", err)
	}
	go func() { _ = cmd.Wait(); logfile.Close() }()
	return waitSnapshot(ctx, client)
}
func (a *App) accept(snap model.Snapshot, force bool) {
	if !a.busy && snap.Status.Error != "" && snap.Status.Error != a.snapshot.Status.Error {
		a.problem = snap.Status.Error
	}
	if snap.Status.PID != a.snapshot.Status.PID {
		a.traffic = model.Traffic{}
		a.connections = model.Connections{}
		a.groups = nil
		a.connectionsError, a.proxiesError = nil, nil
		a.metrics = nil
		a.geoData = nil
	}
	oldGlobal := a.snapshot.State.GlobalOverrideByID(a.loadedGlobal)
	a.snapshot = snap
	a.snapshotVersion++
	if !a.global.Buffer.Modified() {
		if preset := snap.State.GlobalOverrideByID(snap.State.SelectedGlobalOverride); preset != nil {
			if a.loadedGlobal != preset.ID || a.savedGlobal != preset.Content {
				a.global.Buffer.Reset(preset.Content)
				a.global.Buffer.MarkSaved()
				a.loadedGlobal, a.savedGlobal = preset.ID, preset.Content
				a.preview = model.Preview{}
			} else if oldGlobal != nil && oldGlobal.Name != preset.Name {
				a.preview = model.Preview{}
			}
		}
	}
	if a.selected == "" && len(snap.State.Profiles) > 0 {
		a.selected = snap.State.Profiles[0].ID
	}
	p := a.current()
	if p == nil {
		a.loaded = ""
		return
	}
	if force || a.loaded != p.ID {
		a.source.Buffer.Reset(p.Content)
		a.source.Buffer.MarkSaved()
		a.savedSource = p.Content
		a.override.Buffer.Reset(p.Override)
		a.override.Buffer.MarkSaved()
		a.savedOverride = p.Override
		a.loaded = p.ID
		a.preview = model.Preview{}
	} else {
		if !a.source.Buffer.Modified() && a.savedSource != p.Content {
			a.source.Buffer.Reset(p.Content)
			a.source.Buffer.MarkSaved()
			a.savedSource = p.Content
		}
		if !a.override.Buffer.Modified() && a.savedOverride != p.Override {
			a.override.Buffer.Reset(p.Override)
			a.override.Buffer.MarkSaved()
			a.savedOverride = p.Override
		}
	}
	a.source.ReadOnly = p.Kind == "subscription" || a.busy
}
func (a *App) current() *model.Profile {
	for i := range a.snapshot.State.Profiles {
		if a.snapshot.State.Profiles[i].ID == a.selected {
			return &a.snapshot.State.Profiles[i]
		}
	}
	return nil
}
func (a *App) dirty() bool {
	return a.source.Buffer.Modified() || a.global.Buffer.Modified() || a.override.Buffer.Modified()
}
func (a *App) request(method string, params any, success string, done func(json.RawMessage)) {
	if a.busy {
		return
	}
	if a.needsServiceAuthorization(method, params) {
		a.installService(false, a.authorizationKernel(method, params), func() { a.request(method, params, success, done) })
		return
	}
	a.requestHelper(method, params, success, done)
}

// requestHelper sends the request without asking for administrator authorization.
func (a *App) requestHelper(method string, params any, success string, done func(json.RawMessage)) {
	client := a.client
	after := a.logAfter
	paused := a.paused
	a.busy = true
	a.updateTray()
	a.operationStarted = time.Now()
	a.problem = ""
	a.message = pendingMessage(method)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	a.operationCancel = cancel
	debug := os.Getenv("NATIVEJET_DEBUG") == "1"
	if debug {
		log.Printf("helper request: %s", method)
	}
	go func() {
		defer cancel()
		var raw json.RawMessage
		err := client.Call(ctx, method, params, &raw)
		if debug {
			log.Printf("helper result: %s, error=%v", method, err)
		}
		var snap model.Snapshot
		probe, finishProbe := context.WithTimeout(context.Background(), 3*time.Second)
		snapshotErr := client.Call(probe, "snapshot", nil, &snap)
		finishProbe()
		var logs []model.Log
		if err != nil && !paused {
			logProbe, finish := context.WithTimeout(context.Background(), 3*time.Second)
			_ = client.Call(logProbe, "logs", map[string]any{"after": after}, &logs)
			finish()
		}
		a.win.Update(func() {
			if client != a.client || a.installing {
				return
			}
			defer a.updateTray()
			if debug {
				log.Printf("UI result: %s", method)
			}
			a.busy = false
			a.operationCancel = nil
			a.appendLogs(logs)
			if snapshotErr == nil {
				a.connected = true
				a.accept(snap, false)
			}
			if err != nil {
				a.problem = err.Error()
				switch {
				case method == "geo.update" && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)):
					a.problem = "已停止等待 GeoData 更新。内核可能仍在更新，请查看文件状态和内核日志。"
				case errors.Is(err, context.Canceled):
					a.problem = "操作已取消；请以已应用版本和后台日志为准。"
				case errors.Is(err, context.DeadlineExceeded):
					a.problem = "等待后台操作超时；请查看已应用版本和后台日志。"
				}
				a.message = "操作未完成"
				return
			}
			a.message = success
			if done != nil {
				done(raw)
			}
		})
	}()
}
func (a *App) poll(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mygo.RunOnMain(func() {
				if a.quitting || a.polling || a.connecting != nil || a.installing {
					return
				}
				visible := a.win.IsVisible()
				if !visible && a.tray == nil {
					return
				}
				a.polling = true
				client := a.client
				version := a.snapshotVersion
				page := a.page
				inspectGeo := visible && page == geoDataPage
				after := a.logAfter
				paused := a.paused
				go func() {
					probe, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					var snap model.Snapshot
					err := client.Call(probe, "snapshot", nil, &snap)
					var connections model.Connections
					var logs []model.Log
					var groups []model.Proxy
					var connectionsError, proxiesError error
					var geo *geoResult
					if inspectGeo && err == nil && snap.Status.GeoData {
						geo = &geoResult{client: client}
						geo.err = client.Call(probe, "geo.status", nil, &geo.data)
					}
					if visible && err == nil && snap.Status.Running {
						switch page {
						case "连接":
							connectionsError = client.Call(probe, "connections", nil, &connections)
						case proxiesPage:
							proxiesError = client.Call(probe, "proxies", nil, &groups)
						}
					}
					if visible && err == nil && !paused {
						_ = client.Call(probe, "logs", map[string]any{"after": after}, &logs)
					}
					mygo.RunOnMain(func() {
						a.polling = false
						// A poll can finish after a request has already refreshed the snapshot.
						if a.quitting || client != a.client || a.connecting != nil || a.installing || version != a.snapshotVersion {
							return
						}
						if a.win.IsVisible() {
							defer a.win.Invalidate()
						}
						defer a.updateTray()
						if err != nil {
							a.metrics = nil
							a.geoData = nil
							if a.connected {
								a.connected = false
								a.problem = err.Error()
							}
							return
						}
						if !a.connected && !a.busy {
							a.message = "后台服务已连接"
							a.problem = snap.Status.Error
						}
						a.connected = true
						a.accept(snap, false)
						if geo != nil && (geo.err != nil || geo.data.PID == geoPID(snap.Status)) {
							a.geoData = geo
						}
						a.maybeAutoStartCore()
						if !visible {
							return
						}
						switch page {
						case "连接":
							a.connections = connections
							a.connectionsError = connectionsError
						case proxiesPage:
							a.groups = groups
							a.proxiesError = proxiesError
						}
						if !paused {
							a.appendLogs(logs)
						}
					})
				}()
			})
		}
	}
}

func pendingMessage(method string) string {
	switch method {
	case "config.apply":
		return "正在应用配置"
	case "config.restore":
		return "正在恢复配置"
	case "config.preview":
		return "正在生成配置预览"
	case "profile.save", "override.save", "globalOverride.save":
		return "正在保存配置"
	case "globalOverride.select":
		return "正在切换全局覆盖"
	case "globalOverride.delete":
		return "正在删除全局覆盖"
	case "kernel.download":
		return "正在下载内核"
	case "geo.update":
		return "正在等待 Mihomo 更新 GeoData"
	case "core.start", "core.restart":
		return "正在启动内核"
	case "core.stop":
		return "正在停止内核"
	case "proxies.select":
		return "正在切换策略"
	}
	return "正在处理请求"
}

func (a *App) busyText() string {
	text := a.message
	if !a.operationStarted.IsZero() {
		text += fmt.Sprintf(" · 已用 %d 秒", int(time.Since(a.operationStarted).Seconds()))
	}
	return text
}
func (a *App) loadPreview() {
	if a.current() == nil {
		return
	}
	a.request("config.preview", a.draftParams(), "配置预览已生成", func(raw json.RawMessage) {
		_ = json.Unmarshal(raw, &a.preview)
		a.final.Buffer.Reset(a.preview.YAML)
		a.field = 0
	})
}
func (a *App) draftParams() map[string]string {
	return map[string]string{
		"id":               a.selected,
		"content":          a.source.Buffer.String(),
		"global":           a.global.Buffer.String(),
		"globalOverrideId": a.loadedGlobal,
		"override":         a.override.Buffer.String(),
	}
}
func (a *App) saveSource() {
	p := *a.current()
	p.Content = a.source.Buffer.String()
	a.request("profile.save", map[string]any{"profile": p}, "本地配置已保存，运行配置保持不变", func(_ json.RawMessage) {
		a.source.Buffer.MarkSaved()
		a.savedSource = p.Content
		a.accept(a.snapshot, false)
	})
}
func (a *App) saveOverride() {
	if a.scope == 1 {
		text := a.override.Buffer.String()
		a.request("override.save", map[string]string{"id": a.selected, "text": text}, "覆盖配置已保存，运行配置保持不变", func(_ json.RawMessage) {
			a.override.Buffer.MarkSaved()
			a.savedOverride = text
			a.accept(a.snapshot, false)
		})
		return
	}
	text := a.global.Buffer.String()
	method := "globalOverride.save"
	params := map[string]string{"id": a.loadedGlobal, "content": text}
	if !a.snapshot.Status.GlobalOverrides {
		method = "override.save"
		params = map[string]string{"text": text}
	}
	a.request(method, params, "全局覆盖已保存，点击“应用配置”后生效", func(_ json.RawMessage) {
		a.global.Buffer.MarkSaved()
		a.savedGlobal = text
		// A successful save can precede the next successful snapshot poll.
		if a.snapshot.State.SelectedGlobalOverride != a.loadedGlobal {
			a.accept(a.snapshot, false)
		}
	})
}
func (a *App) apply() {
	if a.dirty() {
		a.problem = "请先保存或撤销编辑，再应用配置"
		return
	}
	a.request("config.apply", map[string]string{"id": a.selected}, "配置已应用", func(_ json.RawMessage) { a.loadPreview() })
}
func (a *App) openDirectory() {
	if a.snapshot.Status.KernelDirectory == "" {
		a.problem = "请先连接后台服务"
		return
	}
	path := a.snapshot.Status.KernelDirectory
	go func() {
		if err := mygo.Shell.OpenPath(path); err != nil {
			a.win.Update(func() { a.problem = err.Error() })
		}
	}()
}
func (a *App) exportLogs() {
	lines := make([]string, 0, len(a.logs))
	for _, l := range a.logs {
		lines = append(lines, fmt.Sprintf("%s [%s] [%s] %s", l.Time.Format(time.RFC3339), l.Level, l.Source, l.Message))
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: "导出日志", DefaultPath: "nativejet.log"})
		if err == nil && path != "" {
			err = os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600)
		}
		if err != nil {
			a.win.Update(func() { a.problem = err.Error() })
		}
	}()
}
func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
func value(v any) string {
	b, _ := json.Marshal(v)
	s := string(b)
	if text, ok := v.(string); ok {
		s = text
	}
	if len([]rune(s)) > 100 {
		s = string([]rune(s)[:97]) + "…"
	}
	return s
}
func check(err error) error {
	if err != nil {
		return err
	}
	return errors.New("未选择文件")
}
func mygoContext() context.Context                { return context.Background() }
func writePrivate(path string, data []byte) error { return os.WriteFile(path, data, 0600) }
