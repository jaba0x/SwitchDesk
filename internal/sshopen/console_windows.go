//go:build windows

package sshopen

import (
	"os/exec"
	"syscall"
)

// SwitchDesk has no console of its own, so a console program such as
// ssh.exe needs a new one to show up at all.
const createNewConsole = 0x00000010

func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
}
