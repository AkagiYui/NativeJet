package installation

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type shellExecuteInfo struct {
	size       uint32
	mask       uint32
	window     windows.Handle
	verb       *uint16
	file       *uint16
	parameters *uint16
	directory  *uint16
	show       int32
	instance   windows.Handle
	idList     uintptr
	class      *uint16
	classKey   windows.Handle
	hotKey     uint32
	icon       windows.Handle
	process    windows.Handle
}

func (p *Prepared) Run(remove bool) error {
	file, err := windows.UTF16PtrFromString(p.Helper)
	if err != nil {
		return err
	}
	args := []string{"--install-service", "--payload", p.Directory, "--digest", p.Digest}
	if remove {
		args = append(args, "--remove-service")
	}
	parameters, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return err
	}
	info := shellExecuteInfo{
		mask:       0x40 | 0x100, // SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NOASYNC
		verb:       windows.StringToUTF16Ptr("runas"),
		file:       file,
		parameters: parameters,
	}
	info.size = uint32(unsafe.Sizeof(info))
	shellExecute := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	if ok, _, e := shellExecute.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		return fmt.Errorf("无法获取 Windows 管理员授权: %w", e)
	}
	if info.process == 0 {
		return errors.New("Windows 未启动后台服务安装器")
	}
	defer windows.CloseHandle(info.process)
	if _, err = windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
		return err
	}
	var code uint32
	if err = windows.GetExitCodeProcess(info.process, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("后台服务安装器退出（%d），请检查 Windows 应用程序事件日志", code)
	}
	return nil
}
