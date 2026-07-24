//go:build !windows

package main

import "os/exec"

func hideWindowsProcess(cmd *exec.Cmd) {
	_ = cmd
}
