package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestWarmStartSuccessPath(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	nowFn := func() time.Time { return time.Unix(0, clock.Load()) }
	sleepFn := func(d time.Duration) {
		clock.Add(int64(d))
	}
	started := false
	stopped := false
	notified := false
	seq := NewWarmStartSequencer(Config{
		WarmDrainTimeout:       100 * time.Millisecond,
		WarmCheckpointWait:     20 * time.Millisecond,
		BackendReadyTimeoutSec: 1,
		EventsPath:             "",
	}, NewLogger(t.TempDir()+"/ws.log"), WarmStartHooks{
		Now:             nowFn,
		Sleep:           sleepFn,
		ActiveRuns:      func() int { return 0 },
		CheckpointAcked: func() bool { return true },
		StopBackend: func() error {
			stopped = true
			return nil
		},
		StartBackend: func() (uint32, int, error) {
			started = true
			return 42, 9118, nil
		},
		WaitReady: func(timeout time.Duration) error { return nil },
		NotifyDesktop: func(port int) error {
			notified = true
			if port != 9118 {
				t.Fatalf("port=%d", port)
			}
			return nil
		},
	})
	snap := seq.Run("test_success", func() (int64, int64) { return 1, 2 })
	if snap.Outcome != WarmStartSuccess {
		t.Fatalf("outcome=%s want success detail=%s", snap.Outcome, snap.Detail)
	}
	if snap.Interrupted {
		t.Fatal("success path must not be interrupted")
	}
	if !stopped || !started || !notified {
		t.Fatalf("hooks stopped=%v started=%v notified=%v", stopped, started, notified)
	}
	if !snap.ResumeTraffic {
		t.Fatal("expected resumeTraffic")
	}
}

func TestWarmStartInterruptedNeverSuccess(t *testing.T) {
	var clock atomic.Int64
	clock.Store(1_000_000_000)
	nowFn := func() time.Time { return time.Unix(0, clock.Load()) }
	sleepFn := func(d time.Duration) { clock.Add(int64(d)) }
	seq := NewWarmStartSequencer(Config{
		WarmDrainTimeout:   50 * time.Millisecond,
		WarmCheckpointWait: 10 * time.Millisecond,
	}, NewLogger(t.TempDir()+"/ws2.log"), WarmStartHooks{
		Now:             nowFn,
		Sleep:           sleepFn,
		ActiveRuns:      func() int { return 3 }, // never drains
		CheckpointAcked: func() bool { return false },
		StopBackend:     func() error { return nil },
		StartBackend:    func() (uint32, int, error) { return 7, 9118, nil },
		WaitReady:       func(timeout time.Duration) error { return nil },
		NotifyDesktop:   func(port int) error { return nil },
	})
	snap := seq.Run("test_interrupted", func() (int64, int64) { return 2, 3 })
	if snap.Outcome == WarmStartSuccess {
		t.Fatal("interrupted path must never be success")
	}
	if snap.Outcome != WarmStartInterrupted {
		t.Fatalf("outcome=%s want interrupted", snap.Outcome)
	}
	if !snap.Interrupted {
		t.Fatal("expected interrupted flag")
	}
	if snap.ActiveRunsAtEnd != 3 {
		t.Fatalf("activeRunsAtEnd=%d", snap.ActiveRunsAtEnd)
	}
}

func TestWarmStartFailedOnStart(t *testing.T) {
	seq := NewWarmStartSequencer(Config{
		WarmDrainTimeout:   time.Millisecond,
		WarmCheckpointWait: time.Millisecond,
	}, NewLogger(t.TempDir()+"/ws3.log"), WarmStartHooks{
		Now:             time.Now,
		Sleep:           func(d time.Duration) {},
		ActiveRuns:      func() int { return 0 },
		CheckpointAcked: func() bool { return true },
		StopBackend:     func() error { return nil },
		StartBackend:    func() (uint32, int, error) { return 0, 0, errStartBoom },
		WaitReady:       func(timeout time.Duration) error { return nil },
	})
	snap := seq.Run("boom", nil)
	if snap.Outcome != WarmStartFailed {
		t.Fatalf("outcome=%s", snap.Outcome)
	}
}

var errStartBoom = errString("start boom")

type errString string

func (e errString) Error() string { return string(e) }
