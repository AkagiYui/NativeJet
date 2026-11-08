package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const ForceStopOnInterruptFailure = true

func SamePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func CurrentSID() (string, error) {
	return tokenSID(windows.GetCurrentProcessToken())
}

func ProcessSID(pid uint32) (string, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(process)
	var token windows.Token
	if err = windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return "", err
	}
	defer token.Close()
	return tokenSID(token)
}

func tokenSID(token windows.Token) (string, error) {
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func ProcessIdentity(pid int) (string, int64, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return "", 0, err
	}
	defer windows.CloseHandle(process)
	wait, err := windows.WaitForSingleObject(process, 0)
	if err != nil {
		return "", 0, err
	}
	if wait != uint32(windows.WAIT_TIMEOUT) {
		return "", 0, os.ErrProcessDone
	}
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil {
		return "", 0, err
	}
	path := make([]uint16, 32768)
	size := uint32(len(path))
	if err = windows.QueryFullProcessImageName(process, 0, &path[0], &size); err != nil {
		return "", 0, err
	}
	return windows.UTF16ToString(path[:size]), created.Nanoseconds(), nil
}

// Join a job before creating any children. Windows adds child processes to the
// job at creation and closes its last handle if the helper dies unexpectedly.
func SuperviseChildren() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, windows.CurrentProcess())
	}
	if err != nil {
		_ = windows.CloseHandle(job)
	}
	return err
}

func ConfigureCoreCommand(cmd *exec.Cmd, home string) {
	root, _ := windows.GetWindowsDirectory()
	cmd.Env = []string{
		"SystemRoot=" + root,
		"WINDIR=" + root,
		"PATH=" + filepath.Join(root, "System32"),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"TEMP=" + home,
		"TMP=" + home,
		"SAFE_PATHS=" + home,
		"LISTEN_NAMEDPIPE_SDDL=D:P(A;;GA;;;OW)(A;;GA;;;SY)(A;;GA;;;BA)",
	}
	if sid, err := CurrentSID(); err == nil {
		cmd.Env[len(cmd.Env)-1] = "LISTEN_NAMEDPIPE_SDDL=D:P(A;;GA;;;" + sid + ")"
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE, HideWindow: true}
}

// The broker joins the core's private console without detaching the helper
// from any console or delivering a control event to the helper itself.
func InterruptCore(process *os.Process, path string, started int64) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "--interrupt-core", strconv.Itoa(process.Pid), path, strconv.FormatInt(started, 10))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func SendCoreInterrupt(args []string) error {
	if len(args) != 3 {
		return errors.New("invalid interrupt arguments")
	}
	pid, err := strconv.Atoi(args[0])
	if err != nil || pid <= 0 {
		return errors.New("invalid process ID")
	}
	stamp, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return err
	}
	path, current, err := ProcessIdentity(pid)
	if err != nil {
		return err
	}
	if !SamePath(path, args[1]) || current != stamp {
		return errors.New("process identity changed")
	}
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	detach := kernel.NewProc("FreeConsole")
	// CREATE_NO_WINDOW can leave this process attached to an invisible
	// console. AttachConsole rejects an already attached process.
	if ok, _, e := detach.Call(); ok == 0 {
		return fmt.Errorf("FreeConsole: %w", e)
	}
	attach := kernel.NewProc("AttachConsole")
	if ok, _, e := attach.Call(uintptr(pid)); ok == 0 {
		return fmt.Errorf("AttachConsole: %w", e)
	}
	received := make(chan struct{}, 1)
	handler := windows.NewCallback(func(event uint32) uintptr {
		if event != windows.CTRL_BREAK_EVENT {
			return 0
		}
		select {
		case received <- struct{}{}:
		default:
		}
		return 1
	})
	if ok, _, e := kernel.NewProc("SetConsoleCtrlHandler").Call(handler, 1); ok == 0 {
		return fmt.Errorf("SetConsoleCtrlHandler: %w", e)
	}
	// Recheck after attaching to guard against a PID being reused.
	path, current, err = ProcessIdentity(pid)
	if err != nil || !SamePath(path, args[1]) || current != stamp {
		return errors.New("process identity changed")
	}
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, 0); err != nil {
		return fmt.Errorf("GenerateConsoleCtrlEvent: %w", err)
	}
	// Delivery is asynchronous. Stay attached until this broker has also
	// handled the broadcast; the OS releases the console when it exits.
	select {
	case <-received:
	case <-time.After(time.Second):
	}
	return nil
}
