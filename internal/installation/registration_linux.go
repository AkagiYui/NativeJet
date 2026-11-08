package installation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"nativejet/internal/buildinfo"
)

const (
	HelperPath       = "/opt/nativejet/nativejet-helper"
	DataRoot         = "/var/lib/nativejet"
	RegistrationPath = DataRoot + "/service.json"
	serviceUnit      = buildinfo.HelperID + ".service"
)

type Registration struct {
	Version           int      `json:"version"`
	OwnerUID          int      `json:"ownerUid"`
	OwnerGID          int      `json:"ownerGid"`
	ApprovedKernels   []string `json:"approvedKernels"`
	HelperRequirement string   `json:"-"`
}

func (r Registration) Data() string {
	return filepath.Join(DataRoot, "users", strconv.Itoa(r.OwnerUID))
}

func (r Registration) Inbox() string {
	return filepath.Join(DataRoot, "inboxes", strconv.Itoa(r.OwnerUID))
}

func (r Registration) VerifyOwner() error {
	if os.Getuid() != r.OwnerUID {
		return errors.New("后台服务已授权给另一位 Linux 用户")
	}
	return nil
}

func LoadRegistration() (Registration, error) {
	var r Registration
	if err := checkOwned(DataRoot, 0, true); err != nil {
		return r, err
	}
	if err := checkOwned(RegistrationPath, 0, false); err != nil {
		return r, err
	}
	root, err := os.OpenRoot(DataRoot)
	if err != nil {
		return r, err
	}
	defer root.Close()
	data, err := readRegular(root, filepath.Base(RegistrationPath), 1<<20)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(data, &r); err != nil {
		return r, err
	}
	if r.Version != 1 || r.OwnerUID <= 0 || r.OwnerGID < 0 {
		return r, errors.New("后台服务注册身份无效")
	}
	for _, id := range r.ApprovedKernels {
		if !digestPattern.MatchString(id) {
			return r, errors.New("内核授权记录无效")
		}
	}
	return r, nil
}

func (r Registration) VerifyServiceDirectories() error {
	for _, path := range []string{DataRoot, filepath.Dir(r.Data()), r.Data(), filepath.Dir(r.Inbox())} {
		if err := checkOwned(path, 0, true); err != nil {
			return err
		}
	}
	return checkOwned(r.Inbox(), r.OwnerUID, true)
}

func checkOwned(path string, uid int, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(uid) || info.Mode().Perm()&0022 != 0 ||
		(directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		return fmt.Errorf("文件或目录的所有者、类型或权限不正确: %s", path)
	}
	return nil
}

func verifyPackagedHelper() error {
	for _, dir := range []string{"/opt", filepath.Dir(HelperPath)} {
		if err := checkOwned(dir, 0, true); err != nil {
			return err
		}
	}
	return checkOwned(HelperPath, 0, false)
}
