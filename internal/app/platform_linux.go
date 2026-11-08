package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"nativejet/internal/platform"
)

const platformName = "Linux"
const trayName = "系统托盘"
const serviceInstallHelp = "通过 Linux 安装包安装 NativeJet 后，可在此授权 systemd 后台服务以使用 TUN。"
const controllerExample = "external-controller-unix: mihomo.sock"
const linkError = "无法注册 Clash 链接，请检查桌面环境的默认应用设置"

func userConfigDir() (string, error) { return os.UserConfigDir() }

func bundledHelper(exe string) string {
	return filepath.Join(filepath.Dir(exe), "nativejet-helper")
}

func developmentRequirement(string) string { return "" }
func configureHelperCommand(*exec.Cmd)     {}

func signingRequirement(string) (string, error) {
	return "", errors.New("Linux TUN 需要已授权的 systemd 后台服务")
}

func developmentEndpoint(data string) string {
	sum := sha256.Sum256([]byte(data))
	name := fmt.Sprintf("%x.sock", sum[:8])
	if runtime := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(runtime) {
		path := filepath.Join(runtime, "nativejet", name)
		if len(path) < 108 {
			return path
		}
	}
	return filepath.Join("/tmp", fmt.Sprintf("nativejet-%d", os.Getuid()), name)
}

func loginItem(action string) (platform.LoginItemStatus, error) {
	var err error
	switch action {
	case "register":
		err = mygo.App.SetOpenAtLogin(true)
	case "unregister":
		err = mygo.App.SetOpenAtLogin(false)
	case "settings":
		err = errors.New("请在当前桌面环境的启动应用设置中管理登录项")
	}
	if mygo.App.OpenAtLogin() {
		return platform.LoginItemOn, err
	}
	return platform.LoginItemOff, err
}

func desktopTrayAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	for _, host := range []string{"org.kde.StatusNotifierWatcher", "org.freedesktop.StatusNotifierWatcher"} {
		out, err := exec.CommandContext(ctx, "/usr/bin/gdbus", "call", "--session", "--dest", host,
			"--object-path", "/StatusNotifierWatcher", "--method", "org.freedesktop.DBus.Properties.Get",
			host, "IsStatusNotifierHostRegistered").Output()
		if err == nil && strings.TrimSpace(string(out)) == "(<true>,)" {
			return true
		}
	}
	return false
}
