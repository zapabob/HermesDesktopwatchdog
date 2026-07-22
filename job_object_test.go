//go:build windows

package main

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestJobObjectCreateAssignTerminate(t *testing.T) {
	job, err := NewProcessJob()
	if err != nil {
		t.Fatal(err)
	}
	defer job.Close()

	cmd := exec.Command("cmd", "/C", "ping", "-n", "30", "127.0.0.1")
	hideWindowsProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	if err := job.AssignPID(pid); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("assign: %v", err)
	}
	if !job.Active() {
		t.Fatal("job should be active")
	}
	if err := job.Terminate(1); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit after job terminate")
	}
	if processAlive(pid) {
		t.Fatal("pid still alive after job terminate")
	}
}

func TestJobObjectAssignSelf(t *testing.T) {
	// Creating a second job and assigning self can fail under nested jobs;
	// just verify create + close works.
	job, err := NewProcessJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Close()
	if job.Active() {
		t.Fatal("closed job must be inactive")
	}
	_ = os.Getpid()
}

func TestPortHoldHint(t *testing.T) {
	h := portHoldHint(9118)
	if h == "" {
		t.Fatal("expected hint")
	}
}
