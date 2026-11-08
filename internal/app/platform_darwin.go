package app

import (
	"crypto/sha256"
	"encoding/hex"
	"nativejet/internal/platform"
	"os"
	"os/exec"
	"path/filepath"
)

func signingRequirement(path string) (string, error) { return platform.SigningRequirement(path) }

const platformName = "macOS"
const trayName = "菜单栏"
const serviceInstallHelp = "安装并授权系统服务后即可使用 TUN。安装需要 macOS 管理员密码。"
const controllerExample = "external-controller-unix: mihomo.sock"
const linkError = "无法设置 Clash 链接的默认应用，请从打包的 NativeJet.app 重试"

func userConfigDir() (string, error) { return os.UserConfigDir() }
func bundledHelper(exe string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(exe)), "Library/LaunchServices/nativejet-helper")
}
func developmentRequirement(string) string                      { return "" }
func configureHelperCommand(*exec.Cmd)                          {}
func loginItem(action string) (platform.LoginItemStatus, error) { return platform.LoginItem(action) }
func developmentEndpoint(data string) string {
	var socket string
	socket = filepath.Join(data, "helper.sock")
	if len(socket) > 100 {
		sum := sha256.Sum256([]byte(data))
		socket = filepath.Join(os.TempDir(), "nativejet-ui-"+hex.EncodeToString(sum[:6])+".sock")
	}
	return socket
}
