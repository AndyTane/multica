package managedkit

import (
	"errors"
	"golang.org/x/sys/windows"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {
	// Match the daemon launcher: independent of the previous console and job.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 | 0x01000000}
}

func processAt(pid int, expected string) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if windows.QueryFullProcessImageName(h, 0, &buffer[0], &size) != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(windows.UTF16ToString(buffer[:size])), filepath.Clean(expected))
}
func retryProcess(cmd *exec.Cmd, err error) bool {
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		return false
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008}
	return true
}
