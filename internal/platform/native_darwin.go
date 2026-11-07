//go:build darwin && cgo

package platform

/*
#cgo LDFLAGS: -framework Foundation -framework Security -framework ServiceManagement
#include <stdlib.h>
int nj_peer(int fd, const char *requirement, unsigned int *uid, char **error);
int nj_login_item(const char *action, char **error);
long long nj_process(int pid, char *path, int capacity);
char *nj_requirement(const char *path, char **error);
int nj_check_signature(const char *path, const char *requirement, int self, char **error);
int nj_authorized_script(const char *script, char **output, char **error);
int nj_peer_pid(int fd);
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"syscall"
	"unsafe"
)

func Requirement(identifier, team string) (string, error) {
	if !regexp.MustCompile(`^[A-Z0-9]{10}$`).MatchString(team) {
		return "", errors.New("特权服务需要 Developer ID 签名；开发时使用 --dev")
	}
	return fmt.Sprintf(`anchor apple generic and identifier "%s" and certificate leaf[subject.OU] = "%s"`, identifier, team), nil
}
func Peer(fd uintptr, requirement string) (int, error) {
	r := C.CString(requirement)
	defer C.free(unsafe.Pointer(r))
	var uid C.uint
	var message *C.char
	if C.nj_peer(C.int(fd), r, &uid, &message) != 0 {
		return 0, cError(message)
	}
	return int(uid), nil
}

type LoginItemStatus int

const (
	// These values mirror SMAppServiceStatus for registered apps.
	LoginItemOff LoginItemStatus = iota
	LoginItemOn
	LoginItemRequiresApproval
	LoginItemUnavailable LoginItemStatus = -1
)

func LoginItem(action string) (LoginItemStatus, error) {
	a := C.CString(action)
	defer C.free(unsafe.Pointer(a))
	var message *C.char
	status := LoginItemStatus(C.nj_login_item(a, &message))
	if status < 0 {
		return LoginItemUnavailable, cError(message)
	}
	return status, nil
}
func SigningRequirement(path string) (string, error) {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var message *C.char
	result := C.nj_requirement(p, &message)
	if result == nil {
		return "", cError(message)
	}
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result), nil
}
func CheckSignature(path, requirement string) error {
	p, r := C.CString(path), C.CString(requirement)
	defer C.free(unsafe.Pointer(p))
	defer C.free(unsafe.Pointer(r))
	var message *C.char
	if C.nj_check_signature(p, r, 0, &message) != 0 {
		return cError(message)
	}
	return nil
}
func CheckSelfRequirement(requirement string) error {
	r := C.CString(requirement)
	defer C.free(unsafe.Pointer(r))
	var message *C.char
	if C.nj_check_signature(nil, r, 1, &message) != 0 {
		return cError(message)
	}
	return nil
}
func RunAuthorizedScript(script string) (string, error) {
	s := C.CString(script)
	defer C.free(unsafe.Pointer(s))
	var output, message *C.char
	if C.nj_authorized_script(s, &output, &message) != 0 {
		return "", cError(message)
	}
	if output == nil {
		return "", errors.New("系统安装器未返回结果")
	}
	defer C.free(unsafe.Pointer(output))
	return C.GoString(output), nil
}
func ProcessIdentity(pid int) (string, int64, error) {
	b := make([]byte, 4096)
	stamp := int64(C.nj_process(C.int(pid), (*C.char)(unsafe.Pointer(&b[0])), C.int(len(b))))
	if stamp <= 0 {
		return "", 0, errors.New("无法读取内核进程身份")
	}
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return string(b[:n]), stamp, nil
}
func Credentials(uid, gid int) *syscall.SysProcAttr {
	p := &syscall.SysProcAttr{Setpgid: true}
	if os.Geteuid() == 0 && uid != 0 {
		p.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), NoSetGroups: true}
	}
	return p
}
func PeerPID(fd uintptr) (int, error) {
	pid := int(C.nj_peer_pid(C.int(fd)))
	if pid <= 0 {
		return 0, errors.New("无法读取 helper 连接进程身份")
	}
	return pid, nil
}
func cError(p *C.char) error {
	if p == nil {
		return errors.New("macOS 操作失败")
	}
	defer C.free(unsafe.Pointer(p))
	return errors.New(C.GoString(p))
}
