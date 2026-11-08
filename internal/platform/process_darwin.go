package platform

import (
	"os"
	"os/exec"
	"syscall"
)

const ForceStopOnInterruptFailure = false

func SamePath(a, b string) bool { return a == b }

func ConfigureCoreCommand(cmd *exec.Cmd, home string) {
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + home, "SAFE_PATHS=" + home}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func InterruptCore(process *os.Process, _ string, _ int64) error {
	return process.Signal(syscall.SIGTERM)
}
