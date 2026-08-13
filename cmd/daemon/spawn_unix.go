//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach 使新 daemon 脱离当前进程组（父进程退出不影响新进程；Unix：setsid）。
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
