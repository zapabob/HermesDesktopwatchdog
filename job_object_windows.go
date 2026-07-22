//go:build windows

package main

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProcessJob wraps a Windows Job Object used to group the managed backend
// process tree so TerminateJobObject reaps children (REQ-LM-08).
type ProcessJob struct {
	mu     sync.Mutex
	handle windows.Handle
	active bool
}

// NewProcessJob creates a Job Object with KILL_ON_JOB_CLOSE so accidental
// handle leak still kills assigned children when the watchdog exits.
func NewProcessJob() (*ProcessJob, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateJobObject: %w", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("SetInformationJobObject: %w", err)
	}
	return &ProcessJob{handle: h, active: true}, nil
}

// AssignPID opens the process and assigns it to the job.
func (j *ProcessJob) AssignPID(pid int) error {
	if j == nil {
		return fmt.Errorf("nil job")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.active || j.handle == 0 {
		return fmt.Errorf("job not active")
	}
	const access = windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE | windows.PROCESS_QUERY_INFORMATION
	ph, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("OpenProcess pid=%d: %w", pid, err)
	}
	defer windows.CloseHandle(ph)
	if err := windows.AssignProcessToJobObject(j.handle, ph); err != nil {
		return fmt.Errorf("AssignProcessToJobObject pid=%d: %w", pid, err)
	}
	return nil
}

// Terminate kills every process in the job (preferred over bare taskkill).
func (j *ProcessJob) Terminate(exitCode uint32) error {
	if j == nil {
		return fmt.Errorf("nil job")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.active || j.handle == 0 {
		return fmt.Errorf("job not active")
	}
	if err := windows.TerminateJobObject(j.handle, exitCode); err != nil {
		return err
	}
	return nil
}

// Close releases the job handle. With KILL_ON_JOB_CLOSE, remaining members die.
func (j *ProcessJob) Close() {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.handle != 0 {
		_ = windows.CloseHandle(j.handle)
		j.handle = 0
	}
	j.active = false
}

// Active reports whether the job handle is still open.
func (j *ProcessJob) Active() bool {
	if j == nil {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.active && j.handle != 0
}
