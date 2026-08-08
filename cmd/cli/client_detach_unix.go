//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// setDaemonDetached 使 daemon 脱离当前会话（Setsid），CLI 退出后 daemon 继续常驻。
func setDaemonDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
