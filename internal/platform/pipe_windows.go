package platform

import (
	"context"
	"errors"
	"net"
	"strings"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func ValidPipe(name string) bool {
	suffix, ok := strings.CutPrefix(strings.ToLower(name), `\\.\pipe\`)
	return ok && suffix != "" && !strings.ContainsAny(suffix, `\/:`)
}

func ListenPipe(name, owner string) (net.Listener, error) {
	if !ValidPipe(name) {
		return nil, errors.New("本地命名管道名称无效")
	}
	sid, err := windows.StringToSid(owner)
	if err != nil {
		return nil, err
	}
	// FILE_WRITE_DATA permits requests but excludes FILE_CREATE_PIPE_INSTANCE.
	access := "0x12008b"
	current, err := CurrentSID()
	if err != nil {
		return nil, err
	}
	if current == sid.String() {
		// An ordinary development helper also needs to create server instances.
		access = "GA"
	}
	acl := "D:P(A;;GA;;;SY)(A;;" + access + ";;;" + sid.String() + ")"
	return winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: acl})
}

func DialPipe(ctx context.Context, name string) (net.Conn, error) {
	if !ValidPipe(name) {
		return nil, errors.New("本地命名管道名称无效")
	}
	return winio.DialPipeAccessImpLevel(ctx, name, windows.FILE_GENERIC_READ|windows.FILE_WRITE_DATA, winio.PipeImpLevelIdentification)
}

func PipeServerPID(conn net.Conn) (uint32, error) {
	file, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return 0, errors.New("无法读取命名管道身份")
	}
	var pid uint32
	err := windows.GetNamedPipeServerProcessId(windows.Handle(file.Fd()), &pid)
	return pid, err
}

func ServicePID(name string) (uint32, error) {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, err
	}
	defer windows.CloseServiceHandle(manager)
	serviceName, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	service, err := windows.OpenService(manager, serviceName, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return 0, err
	}
	defer windows.CloseServiceHandle(service)
	var status windows.SERVICE_STATUS_PROCESS
	var needed uint32
	err = windows.QueryServiceStatusEx(service, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &needed)
	return status.ProcessId, err
}
