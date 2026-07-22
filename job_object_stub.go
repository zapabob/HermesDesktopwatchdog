//go:build !windows

package main

import "fmt"

// ProcessJob is a no-op stub outside Windows.
type ProcessJob struct{}

func NewProcessJob() (*ProcessJob, error) {
	return nil, fmt.Errorf("job objects require windows")
}

func (j *ProcessJob) AssignPID(pid int) error {
	return fmt.Errorf("job objects require windows")
}

func (j *ProcessJob) Terminate(exitCode uint32) error {
	return fmt.Errorf("job objects require windows")
}

func (j *ProcessJob) Close() {}

func (j *ProcessJob) Active() bool { return false }
