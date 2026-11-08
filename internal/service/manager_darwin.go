package service

import (
	"fmt"
	"nativejet/internal/platform"
	"os"
	"os/exec"
)

func servicePrivileged(Options) bool { return os.Geteuid() == 0 }
func prepareInbox(o Options) error {
	if os.Geteuid() == 0 && o.UID != 0 {
		for _, command := range [][]string{{"/bin/mkdir", "-p", o.Inbox}, {"/bin/chmod", "700", o.Inbox}} {
			cmd := exec.Command(command[0], command[1:]...)
			cmd.SysProcAttr = platform.Credentials(o.UID, o.GID)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("准备内核目录: %s: %w", out, err)
			}
		}
	} else if err := os.MkdirAll(o.Inbox, 0700); err != nil {
		return err
	}

	return nil
}
