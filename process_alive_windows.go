//go:build windows

package main

import "golang.org/x/sys/windows"

// processAlive uses OpenProcess instead of tasklist.exe — tasklist can hang
// indefinitely under contention (Job Object tests, antivirus, WMI locks).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	const access = windows.PROCESS_QUERY_LIMITED_INFORMATION
	h, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	const stillActive = 259
	return code == stillActive
}
