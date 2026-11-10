//go:build !windows

package sshopen

import "os/exec"

func prepare(*exec.Cmd) {}
