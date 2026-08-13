//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detach 使新 daemon 脱离当前控制台（Windows：新进程组 + 无新窗口）。
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x08000000, // CREATE_NO_WINDOW
	}
}
