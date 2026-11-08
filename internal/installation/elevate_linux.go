package installation

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func (p *Prepared) Run(remove bool) error {
	if err := verifyPackagedHelper(); err != nil {
		return fmt.Errorf("请先安装 NativeJet 的 .deb 包，再授权系统后台服务: %w", err)
	}
	args := []string{HelperPath, "--install-service", "--payload", p.Directory, "--digest", p.Digest}
	if remove {
		args = append(args, "--remove-service")
	}
	output, err := exec.Command("/usr/bin/pkexec", args...).CombinedOutput()
	if err == nil {
		return nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && (exit.ExitCode() == 126 || exit.ExitCode() == 127) {
		return errors.New("系统授权已取消或未通过，请确认桌面会话有可用的 polkit 授权代理")
	}
	if detail := strings.TrimSpace(string(output)); detail != "" {
		return fmt.Errorf("systemd 后台服务操作失败: %s", detail)
	}
	return fmt.Errorf("无法调用 polkit 授权: %w", err)
}
