package installation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"nativejet/internal/buildinfo"
)

const (
	HelperPath       = "/Library/PrivilegedHelperTools/dev.nativejet.helper"
	RegistrationPath = HelperPath + ".json"
	LaunchDaemonPath = "/Library/LaunchDaemons/dev.nativejet.helper.plist"
	DataRoot         = "/Library/Application Support/NativeJet/users"
	LogDirectory     = "/Library/Logs/NativeJet"
)

type Registration struct {
	Version           int      `json:"version"`
	OwnerUID          int      `json:"ownerUid"`
	HelperRequirement string   `json:"helperRequirement"`
	HelperSHA256      string   `json:"helperSha256"`
	ApprovedKernels   []string `json:"approvedKernels"`
}

func (r Registration) Validate() error {
	if r.Version != 1 || r.OwnerUID <= 0 ||
		r.HelperRequirement == "" || len(r.HelperRequirement) > 8192 || !digestPattern.MatchString(r.HelperSHA256) {
		return errors.New("后台服务注册信息无效")
	}
	for _, hash := range r.ApprovedKernels {
		if !digestPattern.MatchString(hash) {
			return errors.New("后台服务内核授权记录无效")
		}
	}
	return nil
}

func LoadRegistration() (Registration, error) { return loadRegistration(RegistrationPath, 0) }

func loadRegistration(path string, owner uint32) (Registration, error) {
	var r Registration
	info, err := os.Lstat(path)
	if err != nil {
		return r, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || stat.Uid != owner || info.Mode().Perm()&0022 != 0 {
		return r, errors.New("后台服务注册文件的身份或权限不正确")
	}
	f, err := os.Open(path)
	if err != nil {
		return r, err
	}
	defer f.Close()
	if err = json.NewDecoder(io.LimitReader(f, 64<<10)).Decode(&r); err != nil {
		return r, err
	}
	return r, r.Validate()
}

func UserData(uid int) string { return filepath.Join(DataRoot, fmt.Sprint(uid)) }

func launchDaemon(helper, logDir string) []byte {
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>--installed</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
<key>ExitTimeOut</key><integer>12</integer><key>Umask</key><integer>63</integer>
<key>ProcessType</key><string>Background</string>
<key>StandardOutPath</key><string>%s/helper.log</string>
<key>StandardErrorPath</key><string>%s/helper.log</string>
</dict></plist>`, buildinfo.HelperID, helper, logDir, logDir))
}

func (Registration) VerifyOwner() error { return nil }
