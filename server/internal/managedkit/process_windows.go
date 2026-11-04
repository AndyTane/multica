package managedkit

import (
	"errors"
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {
	// Match the daemon launcher: independent of the previous console and job.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 | 0x01000000}
}
func retryProcess(cmd *exec.Cmd, err error) bool {
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		return false
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008}
	return true
}
