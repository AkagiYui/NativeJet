package installation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"nativejet/internal/buildinfo"
	"nativejet/internal/kernel"
	"nativejet/internal/platform"
	"nativejet/internal/storage"
)

func Install(directory, digest string, remove bool) (err error) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("安装 Windows 后台服务需要管理员授权")
	}
	if err = errors.Join(programFilesErr, programDataErr); err != nil {
		return err
	}
	payload, source, err := readPayload(directory, digest)
	if err != nil {
		return err
	}
	defer source.Close()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	hash, err := kernel.Hash(exe)
	if err != nil || hash != payload.HelperSHA256 {
		return errors.New("授权后安装器已改变，拒绝安装")
	}
	if err = protectDirectory(installRoot, installACL); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(installRoot, "install.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	var overlap windows.Overlapped
	if err = windows.LockFileEx(windows.Handle(lock.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlap); err != nil {
		return fmt.Errorf("已有后台服务安装操作在运行: %w", err)
	}
	defer windows.UnlockFileEx(windows.Handle(lock.Fd()), 0, 1, 0, &overlap)

	old, loadErr := LoadRegistration()
	if loadErr == nil && old.OwnerSID != payload.OwnerSID {
		return errors.New("后台服务已授权给另一位 Windows 用户")
	}
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		return loadErr
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(buildinfo.HelperID)
	if err != nil && !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	defer func() {
		if service != nil {
			_ = service.Close()
		}
	}()
	var oldConfig mgr.Config
	var wasRunning bool
	if service != nil {
		oldConfig, err = service.Config()
		if err != nil {
			return err
		}
		if loadErr != nil || !platform.SamePath(oldConfig.BinaryPathName, serviceCommand(old.HelperPath())) {
			return errors.New("同名 Windows 服务的安装身份不匹配")
		}
		status, queryErr := service.Query()
		if queryErr != nil {
			return queryErr
		}
		wasRunning = status.State == svc.Running || status.State == svc.StartPending
	}
	if remove {
		if service != nil {
			if err = stopService(service); err != nil {
				return err
			}
			if err = service.Delete(); err != nil {
				return err
			}
		}
		if loadErr == nil {
			if err = os.Remove(RegistrationPath); err != nil {
				return err
			}
			_ = os.Remove(old.HelperPath())
		}
		return nil
	}

	next := Registration{Version: 1, OwnerSID: payload.OwnerSID, HelperSHA256: hash, ApprovedKernels: slices.Clone(old.ApprovedKernels)}
	for _, dir := range []struct{ path, acl string }{
		{dataRoot, dataRootACL}, {filepath.Join(dataRoot, "users"), privateACL},
		{next.Data(), privateACL}, {next.Inbox(), next.inboxACL()},
	} {
		if err = protectDirectory(dir.path, dir.acl); err != nil {
			return err
		}
	}
	if payload.State != nil {
		if loadErr == nil {
			return errors.New("已安装服务只能使用现有状态")
		}
		if _, err = migrate(payload, source, next.Data()); err != nil {
			return err
		}
	}
	if id := payload.ApproveKernel; id != "" && !slices.Contains(next.ApprovedKernels, id) {
		path := filepath.Join(next.Data(), "cores", id, kernel.Executable)
		if got, e := kernel.Hash(path); e != nil || got != id {
			return errors.New("待授权的内核副本不存在或摘要不符")
		}
		next.ApprovedKernels = append(next.ApprovedKernels, id)
	}
	if got, e := kernel.Hash(next.HelperPath()); e != nil || got != hash {
		data, readErr := os.ReadFile(exe)
		if readErr != nil {
			return readErr
		}
		if err = storage.Atomic(next.HelperPath(), data, 0700); err != nil {
			return err
		}
		if got, e = kernel.Hash(next.HelperPath()); e != nil || got != hash {
			return errors.New("后台服务安装副本摘要不匹配")
		}
	}
	if service != nil {
		if err = stopService(service); err != nil {
			return err
		}
	}
	created := false
	defer func() {
		if err == nil {
			return
		}
		var recovery error
		if service != nil {
			recovery = stopService(service)
			if created {
				recovery = errors.Join(recovery, service.Delete())
			} else {
				recovery = errors.Join(recovery, service.UpdateConfig(oldConfig))
			}
		}
		if loadErr == nil {
			recovery = errors.Join(recovery, saveRegistration(old))
		} else if e := os.Remove(RegistrationPath); e != nil && !errors.Is(e, os.ErrNotExist) {
			recovery = errors.Join(recovery, e)
		}
		if wasRunning && !created && service != nil && recovery == nil {
			recovery = startService(service)
		}
		if recovery != nil {
			err = errors.Join(err, fmt.Errorf("恢复旧版后台服务: %w", recovery))
		}
	}()
	if err = saveRegistration(next); err != nil {
		return err
	}
	if service == nil {
		service, err = manager.CreateService(buildinfo.HelperID, next.HelperPath(), mgr.Config{
			DisplayName: "NativeJet 后台服务", Description: "运行经管理员授权的 NativeJet Mihomo 内核",
			StartType: mgr.StartAutomatic, ServiceStartName: "LocalSystem",
		}, "--installed")
		if err != nil {
			return err
		}
		created = true
	} else if err = windows.ChangeServiceConfig(service.Handle, windows.SERVICE_NO_CHANGE, windows.SERVICE_NO_CHANGE,
		windows.SERVICE_NO_CHANGE, windows.StringToUTF16Ptr(serviceCommand(next.HelperPath())), nil, nil, nil, nil, nil, nil); err != nil {
		return err
	}
	if err = startService(service); err != nil {
		return err
	}
	if loadErr == nil && old.HelperSHA256 != next.HelperSHA256 {
		_ = os.Remove(old.HelperPath())
	}
	return nil
}

func serviceCommand(path string) string {
	return windows.EscapeArg(path) + " --installed"
}

func saveRegistration(r Registration) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return storage.Atomic(RegistrationPath, data, 0600)
}

func startService(service *mgr.Service) error {
	if err := service.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return err
	}
	return waitService(service, svc.Running)
}

func stopService(service *mgr.Service) error {
	status, err := service.Query()
	if err != nil || status.State == svc.Stopped {
		return err
	}
	if status.State == svc.StartPending {
		if err = waitService(service, svc.Running); err != nil {
			status, queryErr := service.Query()
			if queryErr == nil && status.State == svc.Stopped {
				return nil
			}
			return err
		}
		status.State = svc.Running
	}
	if status.State != svc.StopPending {
		if _, err = service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
			return err
		}
	}
	return waitService(service, svc.Stopped)
}

func waitService(service *mgr.Service, want svc.State) error {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		status, err := service.Query()
		if err != nil || status.State == want {
			return err
		}
		if status.State == svc.Stopped {
			return fmt.Errorf("后台服务已退出（Windows %d / 服务 %d）", status.Win32ExitCode, status.ServiceSpecificExitCode)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("等待 Windows 后台服务超时")
}
