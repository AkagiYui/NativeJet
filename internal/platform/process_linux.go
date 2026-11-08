package platform

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

func ProcessIdentity(pid int) (string, int64, error) {
	if pid <= 0 {
		return "", 0, os.ErrProcessDone
	}
	dir := filepath.Join("/proc", strconv.Itoa(pid))
	started, err := processStart(dir)
	if err != nil {
		return "", 0, err
	}
	path, err := os.Readlink(filepath.Join(dir, "exe"))
	if err != nil {
		return "", 0, err
	}
	current, err := processStart(dir)
	if err != nil || current != started {
		return "", 0, os.ErrProcessDone
	}
	return path, started, nil
}

func processStart(dir string) (int64, error) {
	data, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return 0, err
	}
	// comm is enclosed in parentheses and can itself contain a closing
	// parenthesis. Field 22 (starttime) is 19 fields after comm.
	_, tail, ok := strings.Cut(string(data), "(")
	fields := strings.Fields(tail[strings.LastIndexByte(tail, ')')+1:])
	if !ok || len(fields) <= 19 {
		return 0, errors.New("无法读取进程启动时间")
	}
	if fields[0] == "Z" || fields[0] == "X" {
		return 0, os.ErrProcessDone
	}
	return strconv.ParseInt(fields[19], 10, 64)
}
