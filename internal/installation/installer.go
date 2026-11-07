package installation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"
	"time"

	"nativejet/internal/buildinfo"
	"nativejet/internal/kernel"
	"nativejet/internal/model"
	"nativejet/internal/platform"
	"nativejet/internal/storage"
)

type Result struct {
	Error string `json:"error,omitempty"`
}

func control(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/launchctl", args...)
	// Authorization Services can preserve the requesting user's real UID.
	// launchctl must run with both real and effective root credentials.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Credential: &syscall.Credential{Uid: 0, Gid: 0, NoSetGroups: true}}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("后台服务操作失败: %s: %w", string(out), err)
	}
	return nil
}

type controller func(...string) error

func stopService(run controller, timeout time.Duration) error {
	label := "system/" + buildinfo.HelperID
	if run("print", label) != nil {
		return nil
	}
	if err := run("bootout", label); err != nil {
		return err
	}
	// bootout may return before launchd removes the job registration. Replacing
	// the executable or bootstrapping during that interval is unreliable.
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if run("print", label) != nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("等待旧后台服务卸载超时，配置已保留")
}
func bootstrapService(run controller) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if err = run("bootstrap", "system", LaunchDaemonPath); err == nil {
			return nil
		}
		// Some launchctl failures still register the job; the caller separately
		// verifies the running helper's code identity and readiness.
		if run("print", "system/"+buildinfo.HelperID) == nil {
			return nil
		}
		if attempt < 3 {
			time.Sleep(time.Duration(attempt+1) * 250 * time.Millisecond)
		}
	}
	return err
}

func protectDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !ok || stat.Uid != 0 || info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("系统服务目录身份或权限不正确: %s", path)
	}
	return nil
}

func Install(directory, digest string, remove bool) error {
	if os.Geteuid() != 0 {
		return errors.New("安装必须经过 macOS 管理员授权")
	}
	syscall.Umask(0077)
	payload, root, err := readPayload(directory, digest)
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err = user.LookupId(strconv.Itoa(payload.OwnerUID)); err != nil {
		return errors.New("授权用户不存在")
	}
	old, oldErr := LoadRegistration()
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		return oldErr
	}
	if oldErr == nil && old.OwnerUID != payload.OwnerUID {
		return errors.New("后台服务由另一位用户授权，不能接管该用户的服务")
	}
	if remove {
		return uninstall(old, oldErr)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	hash, err := kernel.Hash(exe)
	if err != nil || hash != payload.HelperSHA256 {
		return errors.New("安装器文件摘要不匹配")
	}
	if err = platform.CheckSignature(exe, payload.HelperRequirement); err != nil {
		return err
	}
	for _, path := range []string{filepath.Dir(HelperPath), filepath.Dir(LaunchDaemonPath), filepath.Dir(DataRoot), DataRoot, LogDirectory} {
		if err = protectDirectory(path); err != nil {
			return err
		}
	}
	// The registration contains public code identities, never configuration or
	// credentials. Its parent must remain traversable by the GUI user.
	if err = os.Chmod(filepath.Dir(HelperPath), 0755); err != nil {
		return err
	}
	// Preserve recoverable copies of the small service registration files.
	backups := map[string][]byte{}
	for _, path := range []string{HelperPath, RegistrationPath, LaunchDaemonPath} {
		data, e := os.ReadFile(path)
		if e == nil {
			backups[path] = data
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
	}
	wasLoaded := control("print", "system/"+buildinfo.HelperID) == nil
	committed := false
	var stateBackup []byte
	statePath := filepath.Join(UserData(payload.OwnerUID), "state.json")
	stateRead := false
	defer func() {
		if committed {
			return
		}
		_ = stopService(control, 25*time.Second)
		for _, path := range []string{HelperPath, RegistrationPath, LaunchDaemonPath} {
			if data, ok := backups[path]; ok {
				mode := os.FileMode(0644)
				if path == HelperPath {
					mode = 0755
				}
				_ = storage.Atomic(path, data, mode)
			} else {
				_ = os.Remove(path)
			}
		}
		if stateRead && stateBackup != nil {
			_ = storage.Atomic(statePath, stateBackup, 0600)
		} else if stateRead {
			_ = os.Remove(statePath)
		}
		if wasLoaded {
			_ = bootstrapService(control)
		}
	}()
	if wasLoaded {
		if err = stopService(control, 25*time.Second); err != nil {
			return err
		}
	}
	stateBackup, err = os.ReadFile(statePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stateRead = true
	state, err := migrate(payload, root, UserData(payload.OwnerUID))
	if err != nil {
		return err
	}
	registration := Registration{Version: 1, OwnerUID: payload.OwnerUID,
		HelperRequirement: payload.HelperRequirement, HelperSHA256: payload.HelperSHA256}
	if oldErr == nil {
		registration.ApprovedKernels = append(registration.ApprovedKernels, old.ApprovedKernels...)
	}
	if payload.ApproveKernel != "" {
		index := slices.IndexFunc(state.Installed, func(k model.Kernel) bool { return k.ID == payload.ApproveKernel })
		if index < 0 {
			return errors.New("找不到待授权内核")
		}
		hash, e := kernel.Hash(state.Installed[index].Path)
		if e != nil || hash != payload.ApproveKernel {
			return errors.New("待授权内核身份已改变")
		}
		if !slices.Contains(registration.ApprovedKernels, hash) {
			registration.ApprovedKernels = append(registration.ApprovedKernels, hash)
		}
	}
	if err = registration.Validate(); err != nil {
		return err
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if err = storage.Atomic(HelperPath, binary, 0755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(registration, "", "  ")
	if err = storage.Atomic(RegistrationPath, data, 0644); err != nil {
		return err
	}
	if err = storage.Atomic(LaunchDaemonPath, launchDaemon(HelperPath, LogDirectory), 0644); err != nil {
		return err
	}
	if err = control("enable", "system/"+buildinfo.HelperID); err != nil {
		return err
	}
	if err = bootstrapService(control); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		conn, e := net.DialTimeout("unix", buildinfo.Socket, 200*time.Millisecond)
		if e == nil {
			raw, rawErr := conn.(*net.UnixConn).SyscallConn()
			var pid int
			var auth error
			if rawErr == nil {
				rawErr = raw.Control(func(fd uintptr) {
					pid, _ = platform.PeerPID(fd)
					_, auth = platform.Peer(fd, registration.HelperRequirement)
				})
			}
			path, _, identityErr := platform.ProcessIdentity(pid)
			conn.Close()
			if rawErr == nil && auth == nil && identityErr == nil && path == HelperPath {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		return errors.New("后台服务未能启动，已恢复原有服务和配置")
	}
	committed = true
	return nil
}

func uninstall(old Registration, oldErr error) error {
	if errors.Is(oldErr, os.ErrNotExist) {
		return nil
	}
	if oldErr != nil {
		return oldErr
	}
	if err := stopService(control, 25*time.Second); err != nil {
		return err
	}
	for _, path := range []string{LaunchDaemonPath, RegistrationPath, HelperPath} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil // private user data is intentionally retained
}
