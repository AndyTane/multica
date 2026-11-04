//go:build !windows

package managedkit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func configureProcess(cmd *exec.Cmd)             {}
func retryProcess(cmd *exec.Cmd, err error) bool { return false }

func processAt(pid int, expected string) bool {
	path, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	return err == nil && filepath.Clean(path) == filepath.Clean(expected)
}
