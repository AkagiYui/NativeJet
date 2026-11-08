package installation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"nativejet/internal/kernel"
	"nativejet/internal/model"
	"nativejet/internal/storage"
)

func control(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func createProtectedDirectory(path string, mode os.FileMode) error {
	if err := os.Mkdir(path, mode); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := checkOwned(path, 0, true); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func (r Registration) prepareDirectories() error {
	for _, entry := range []struct {
		path string
		mode os.FileMode
	}{{filepath.Dir(r.Data()), 0700}, {r.Data(), 0700}, {filepath.Dir(r.Inbox()), 0755}} {
		if err := createProtectedDirectory(entry.path, entry.mode); err != nil {
			return err
		}
	}
	if err := os.Mkdir(r.Inbox(), 0700); err == nil {
		if err = os.Chown(r.Inbox(), r.OwnerUID, r.OwnerGID); err != nil {
			_ = os.Remove(r.Inbox())
			return err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return err
	} else if err = checkOwned(r.Inbox(), 0, true); err == nil {
		// Recover a directory left between mkdir and chown by an interrupted install.
		if err = os.Chown(r.Inbox(), r.OwnerUID, r.OwnerGID); err != nil {
			return err
		}
	}
	return r.VerifyServiceDirectories()
}

func saveRegistration(r Registration) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return storage.Atomic(RegistrationPath, data, 0644)
}

func Install(directory, digest string, remove bool) (err error) {
	if os.Geteuid() != 0 {
		return errors.New("安装必须经过 Linux 管理员授权")
	}
	if err = verifyPackagedHelper(); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil || exe != HelperPath {
		return errors.New("安装器必须从系统安装的 helper 启动")
	}
	payload, source, err := readPayload(directory, digest)
	if err != nil {
		return err
	}
	defer source.Close()
	caller := os.Getenv("PKEXEC_UID")
	if caller == "" {
		caller = os.Getenv("SUDO_UID")
	}
	if caller != strconv.Itoa(payload.OwnerUID) {
		return errors.New("授权调用者与迁移数据所有者不一致")
	}
	account, err := user.LookupId(caller)
	if err != nil {
		return fmt.Errorf("无法读取授权用户: %w", err)
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil || gid < 0 {
		return errors.New("无法读取授权用户的组")
	}
	hash, err := kernel.Hash(HelperPath)
	if err != nil || hash != payload.HelperSHA256 {
		return errors.New("安装器文件摘要不匹配，请从已安装的 NativeJet 打开设置")
	}
	if err = createProtectedDirectory(DataRoot, 0755); err != nil {
		return err
	}
	lock, err := unix.Open(filepath.Join(DataRoot, "install.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer unix.Close(lock)
	if err = unix.Flock(lock, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fmt.Errorf("另一项后台服务操作正在进行: %w", err)
	}
	old, oldErr := LoadRegistration()
	hadRegistration := oldErr == nil
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		return oldErr
	}
	if hadRegistration && old.OwnerUID != payload.OwnerUID {
		return errors.New("后台服务由另一位用户授权，不能接管该用户的服务")
	}
	wasActive := hadRegistration && control("is-active", "--quiet", serviceUnit) == nil
	wasEnabled := hadRegistration && control("is-enabled", "--quiet", serviceUnit) == nil
	if remove {
		if !hadRegistration {
			return nil
		}
		if err = control("disable", "--now", serviceUnit); err == nil {
			err = os.Remove(RegistrationPath)
		}
		if err != nil {
			err = errors.Join(err, restoreService(wasActive, wasEnabled))
		}
		return err
	}

	r := Registration{Version: 1, OwnerUID: payload.OwnerUID, OwnerGID: gid, ApprovedKernels: old.ApprovedKernels}
	if err = r.prepareDirectories(); err != nil {
		return err
	}
	if hadRegistration {
		if err = control("stop", serviceUnit); err != nil {
			return errors.Join(err, restoreService(wasActive, wasEnabled))
		}
	}
	statePath := filepath.Join(r.Data(), "state.json")
	var previousState []byte
	if payload.State != nil {
		previousState, err = os.ReadFile(statePath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.Join(err, restoreService(wasActive, wasEnabled))
		}
	}
	serviceChanged := false
	defer func() {
		if err == nil {
			return
		}
		if serviceChanged {
			err = errors.Join(err, control("stop", serviceUnit))
		}
		if payload.State != nil {
			if previousState != nil {
				err = errors.Join(err, storage.Atomic(statePath, previousState, 0600))
			} else {
				err = errors.Join(err, removeIfPresent(statePath))
			}
		}
		if serviceChanged {
			if hadRegistration {
				err = errors.Join(err, saveRegistration(old))
			} else {
				err = errors.Join(err, removeIfPresent(RegistrationPath))
			}
		}
		if hadRegistration || serviceChanged {
			err = errors.Join(err, restoreService(wasActive, wasEnabled))
		}
	}()

	state, err := migrate(payload, source, r.Data())
	if err != nil {
		return err
	}
	if id := payload.ApproveKernel; id != "" {
		if !slices.ContainsFunc(state.Installed, func(k model.Kernel) bool { return k.ID == id }) {
			return errors.New("找不到待授权内核")
		}
		hash, hashErr := kernel.Hash(filepath.Join(r.Data(), "cores", id, kernel.Executable))
		if hashErr != nil || hash != id {
			return errors.New("待授权内核身份已改变")
		}
		if !slices.Contains(r.ApprovedKernels, id) {
			r.ApprovedKernels = append(r.ApprovedKernels, id)
		}
	}
	if err = saveRegistration(r); err != nil {
		return err
	}
	serviceChanged = true
	return control("enable", "--now", serviceUnit)
}

func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func restoreService(active, enabled bool) error {
	action := "disable"
	if enabled {
		action = "enable"
	}
	err := control(action, serviceUnit)
	if active {
		err = errors.Join(err, control("start", serviceUnit))
	}
	return err
}
