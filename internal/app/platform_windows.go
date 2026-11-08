package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"

	"nativejet/internal/platform"
)

const platformName = "Windows"
const trayName = "系统托盘"
const serviceInstallHelp = "安装并授权 Windows 后台服务后即可使用 TUN。安装会触发 UAC 管理员授权。"
const controllerExample = `external-controller-pipe: '\\.\pipe\NativeJet-Mihomo'`
const linkError = "无法注册 Clash 链接，请检查 Windows 默认应用设置"

func userConfigDir() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
}

func bundledHelper(exe string) string {
	return filepath.Join(filepath.Dir(exe), "nativejet-helper.exe")
}

func developmentEndpoint(data string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(data))))
	return `\\.\pipe\NativeJet-user-` + hex.EncodeToString(sum[:12])
}

func developmentRequirement(helper string) string { return helper }

func configureHelperCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

func signingRequirement(string) (string, error) {
	return "", errors.New("Windows 提权模式使用安装后的后台服务")
}

func loginItem(action string) (platform.LoginItemStatus, error) {
	var err error
	switch action {
	case "register":
		err = mygo.App.SetOpenAtLogin(true)
	case "unregister":
		err = mygo.App.SetOpenAtLogin(false)
	case "settings":
		err = mygo.Shell.OpenExternal("ms-settings:startupapps")
	}
	if mygo.App.OpenAtLogin() {
		return platform.LoginItemOn, err
	}
	return platform.LoginItemOff, err
}
