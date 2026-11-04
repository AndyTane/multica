//go:build !windows

package managedkit

import "os/exec"

func configureProcess(cmd *exec.Cmd)             {}
func retryProcess(cmd *exec.Cmd, err error) bool { return false }
