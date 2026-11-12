package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"nativejet/internal/buildinfo"
	"nativejet/internal/installation"
	"nativejet/internal/ipc"
	"nativejet/internal/model"
	"nativejet/internal/storage"
)

func appRegistration() (installation.Registration, error) {
	r, err := installation.LoadRegistration()
	if err == nil {
		err = r.VerifyOwner()
	}
	return r, err
}

func installedClient(r installation.Registration) *ipc.Client {
	return &ipc.Client{Socket: buildinfo.Socket, Requirement: r.HelperRequirement}
}
func waitSnapshot(ctx context.Context, client *ipc.Client) (model.Snapshot, error) {
	var snap model.Snapshot
	var err error
	for ctx.Err() == nil {
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = client.Call(probe, "snapshot", nil, &snap)
		cancel()
		if err == nil {
			return snap, nil
		}
		select {
		case <-ctx.Done():
		case <-time.After(200 * time.Millisecond):
		}
	}
	return snap, fmt.Errorf("后台服务连接失败: %w", errors.Join(err, ctx.Err()))
}
func (a *App) refreshService() {
	r, err := appRegistration()
	if errors.Is(err, os.ErrNotExist) {
		a.registration = nil
		a.helperError = ""
		return
	}
	if err != nil {
		a.helperError = err.Error()
		return
	}
	a.registration = &r
	a.helperError = ""
	if a.client.Development || a.client.Requirement != r.HelperRequirement {
		a.client = installedClient(r)
		a.connected = false
		a.resetWindowData()
		a.logs, a.logAfter = nil, 0
		a.updateTray()
	}
	if !a.connected {
		a.connect(a.windowContext)
	}
}
func (a *App) authorizationKernel(method string, params any) string {
	if method == "config.restore" {
		var p struct {
			ID string `json:"id"`
		}
		data, _ := json.Marshal(params)
		_ = json.Unmarshal(data, &p)
		for _, r := range a.snapshot.State.Revisions {
			if r.ID == p.ID {
				return r.KernelID
			}
		}
	}
	return a.snapshot.State.SelectedKernel
}
func (a *App) needsServiceAuthorization(method string, params any) bool {
	switch method {
	case "core.start", "core.restart", "config.apply", "config.restore":
	default:
		return false
	}
	id := a.authorizationKernel(method, params)
	if !a.client.Development {
		return id != "" && !slices.Contains(a.snapshot.Status.AuthorizedKernels, id)
	}
	if method != "core.start" && method != "core.restart" {
		return false
	}
	return a.snapshot.Status.ActiveTUN
}

// Installation keeps clients immutable: late replies from the previous helper
// cannot overwrite the new connection or an editor draft.
func (a *App) installService(remove bool, approve string, done func()) {
	if a.busy {
		return
	}
	a.installing = true
	a.busy = true
	a.connected = false
	a.resetWindowData()
	a.previewInput = previewInput{}
	a.updateTray()
	a.problem = ""
	a.operationCancel = nil
	a.operationStarted = time.Now()
	a.message = "正在准备配置备份，随后请在系统弹窗中授权"
	if remove {
		a.message = "正在卸载后台服务并保留配置"
	}
	previous := a.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		var snapshot model.Snapshot
		var err error
		stopped := false
		wasRunning := false
		if previous.Development {
			snapshot, err = a.connectSnapshot(ctx, previous)
			if err == nil {
				wasRunning = snapshot.Status.Running
				if wasRunning {
					err = previous.Call(ctx, "core.stop", nil, nil)
				}
				if err == nil {
					err = previous.Call(ctx, "snapshot", nil, &snapshot)
				}
				if err == nil {
					err = previous.StopDevelopmentHelper(ctx, a.helperPath)
					stopped = err == nil
				}
				if err == nil {
					// Read the final committed state after graceful shutdown. Source files
					// remain in place even if the administrator cancels the next dialog.
					var store *storage.Store
					store, err = storage.New(filepath.Join(a.data, "development"))
					if err == nil {
						snapshot.State, err = store.Load()
					}
				}
			}
		} else {
			probeErr := previous.Call(ctx, "snapshot", nil, &snapshot)
			if remove {
				err = probeErr
			} else if probeErr == nil {
				wasRunning = snapshot.Status.Running
			}
		}
		cancel()
		var prepared *installation.Prepared
		if err == nil {
			var source *model.Snapshot
			if previous.Development && !remove {
				// Repair an installed service without re-importing a stale ordinary-user
				// copy, even if the public registration record could not be read.
				if _, statErr := os.Stat(installation.RegistrationPath); errors.Is(statErr, os.ErrNotExist) {
					source = &snapshot
				} else if statErr != nil {
					err = statErr
				}
			}
			if err == nil {
				prepared, err = installation.Prepare(a.helperPath, source)
			}
		}
		if prepared != nil {
			defer prepared.Close()
		}
		if err == nil && !remove && approve != "" {
			err = prepared.ApproveKernel(approve)
		}
		if err == nil {
			err = prepared.Run(remove)
		}
		next := previous
		var registration *installation.Registration
		var result model.Snapshot
		connected := false
		installed := false
		removed := err == nil && remove
		if err == nil && !remove {
			var r installation.Registration
			r, err = appRegistration()
			if err == nil {
				registration = &r
				next = installedClient(r)
				installed = true
				probe, finish := context.WithTimeout(context.Background(), 20*time.Second)
				result, err = waitSnapshot(probe, next)
				finish()
				connected = err == nil
			}
		} else if err == nil && remove {
			next = a.developmentClient
			probe, finish := context.WithTimeout(context.Background(), 20*time.Second)
			result, err = a.connectSnapshot(probe, next)
			if err == nil {
				err = next.Call(probe, "profiles.import", map[string]any{"state": snapshot.State}, nil)
			}
			if err == nil {
				err = next.Call(probe, "snapshot", nil, &result)
			}
			finish()
			connected = err == nil
		} else if stopped {
			probe, finish := context.WithTimeout(context.Background(), 20*time.Second)
			var recovery error
			result, recovery = a.connectSnapshot(probe, previous)
			if recovery == nil && wasRunning {
				recovery = previous.Call(probe, "core.start", nil, nil)
				if recovery == nil {
					_ = previous.Call(probe, "snapshot", nil, &result)
				}
			}
			finish()
			connected = recovery == nil
			if recovery != nil {
				err = errors.Join(err, fmt.Errorf("恢复普通服务失败: %w", recovery))
			}
		} else {
			probe, finish := context.WithTimeout(context.Background(), 3*time.Second)
			connected = previous.Call(probe, "snapshot", nil, &result) == nil
			finish()
		}
		a.win.Update(func() {
			defer a.updateTray()
			a.installing = false
			a.busy = false
			a.client = next
			a.connected = connected
			if installed {
				a.registration = registration
			}
			if removed {
				a.registration = nil
			}
			a.logs = nil
			a.logAfter = 0
			if connected {
				a.accept(result, false)
			}
			if err != nil {
				a.problem = err.Error()
				a.message = "服务操作未完成；原有配置已保留"
				return
			}
			a.helperError = ""
			a.message = "后台服务已安装并连接，原有配置已保留"
			if remove {
				a.message = "后台服务已卸载，配置已保留，现使用普通用户服务"
			}
			if wasRunning && !remove {
				a.request("core.start", nil, "后台服务已更新，内核已恢复运行", func(json.RawMessage) {
					if done != nil {
						done()
					}
				})
			} else if done != nil {
				done()
			}
		})
	}()
}
