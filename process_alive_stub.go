//go:build !windows

package main

import (
	"os"
	"syscall"
)

// processAlive is a best-effort check outside Windows (source builds only).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
