package main

import (
	"testing"
	"time"
)

func TestRestartTrackerBackoffAndFailed(t *testing.T) {
	// T09-oriented: exponential backoff then Failed latch.
	p := RestartPolicy{
		MaxRestarts:    3,
		Window:         10 * time.Minute,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     8 * time.Second,
		ResetAfter:     10 * time.Minute,
	}
	tr := NewRestartTracker(p)
	base := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

	b1, fail1 := tr.RecordFailure(base)
	if fail1 || b1 != 1*time.Second {
		t.Fatalf("attempt1 backoff=%v failed=%v", b1, fail1)
	}
	if tr.CanAttempt(base.Add(500 * time.Millisecond)) {
		t.Fatal("should still be in backoff")
	}
	if !tr.CanAttempt(base.Add(1 * time.Second)) {
		t.Fatal("should allow after backoff")
	}

	b2, _ := tr.RecordFailure(base.Add(2 * time.Second))
	if b2 != 2*time.Second {
		t.Fatalf("attempt2 backoff=%v want 2s", b2)
	}
	b3, fail3 := tr.RecordFailure(base.Add(5 * time.Second))
	if !fail3 {
		t.Fatal("expected Failed latch on 3rd attempt")
	}
	if b3 != 4*time.Second {
		t.Fatalf("attempt3 backoff=%v want 4s", b3)
	}
	if tr.CanAttempt(base.Add(1 * time.Hour)) {
		t.Fatal("Failed must block all auto attempts")
	}

	tr.ClearFailed()
	if !tr.CanAttempt(base.Add(1 * time.Hour)) {
		t.Fatal("ClearFailed should re-enable attempts")
	}
}

func TestRestartTrackerMarkReadyClears(t *testing.T) {
	tr := NewRestartTracker(DefaultRestartPolicy())
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	tr.RecordFailure(now)
	tr.MarkReady(now.Add(time.Second))
	if tr.Failed() || tr.AttemptCount() != 0 {
		t.Fatalf("MarkReady should clear tracker failed=%v attempts=%d", tr.Failed(), tr.AttemptCount())
	}
}

func TestRestartTrackerWindowPrune(t *testing.T) {
	p := RestartPolicy{
		MaxRestarts:    2,
		Window:         1 * time.Minute,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Second,
		ResetAfter:     10 * time.Minute,
	}
	tr := NewRestartTracker(p)
	t0 := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	tr.RecordFailure(t0)
	// Outside window — should not latch on second failure after prune.
	_, failed := tr.RecordFailure(t0.Add(2 * time.Minute))
	if failed {
		t.Fatal("old attempt should be pruned; must not latch Failed")
	}
	if tr.AttemptCount() != 1 {
		t.Fatalf("attempts=%d want 1", tr.AttemptCount())
	}
}

func TestRestartTrackerMaxBackoffCap(t *testing.T) {
	p := RestartPolicy{
		MaxRestarts:    10,
		Window:         time.Hour,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     3 * time.Second,
		ResetAfter:     time.Hour,
	}
	tr := NewRestartTracker(p)
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	var last time.Duration
	for i := 0; i < 5; i++ {
		last, _ = tr.RecordFailure(now.Add(time.Duration(i) * 10 * time.Second))
	}
	if last != 3*time.Second {
		t.Fatalf("backoff capped at max got %v", last)
	}
}
