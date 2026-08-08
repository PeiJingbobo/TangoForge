//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// setDaemonDetached 使 daemon 脱离当前控制台（DETACHED_PROCESS），CLI 退出后继续常驻。
func setDaemonDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | createNewProcessGroup}
}
