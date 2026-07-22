package main

import "time"

// RestartPolicy limits automatic recovery (ADR REQ-LM-05).
type RestartPolicy struct {
	MaxRestarts    int
	Window         time.Duration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	ResetAfter     time.Duration
}

// DefaultRestartPolicy matches the ADR defaults.
func DefaultRestartPolicy() RestartPolicy {
	return RestartPolicy{
		MaxRestarts:    5,
		Window:         10 * time.Minute,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     60 * time.Second,
		ResetAfter:     10 * time.Minute,
	}
}

// RestartTracker tracks failures with monotonic timestamps (time.Time from a clock).
type RestartTracker struct {
	policy   RestartPolicy
	attempts []time.Time
	lastOK   time.Time
	nextAt   time.Time
	failed   bool
	attemptN int // consecutive failure streak for backoff exponent
}

func NewRestartTracker(p RestartPolicy) *RestartTracker {
	if p.MaxRestarts <= 0 {
		p.MaxRestarts = DefaultRestartPolicy().MaxRestarts
	}
	if p.Window <= 0 {
		p.Window = DefaultRestartPolicy().Window
	}
	if p.InitialBackoff <= 0 {
		p.InitialBackoff = DefaultRestartPolicy().InitialBackoff
	}
	if p.MaxBackoff <= 0 {
		p.MaxBackoff = DefaultRestartPolicy().MaxBackoff
	}
	if p.ResetAfter <= 0 {
		p.ResetAfter = DefaultRestartPolicy().ResetAfter
	}
	return &RestartTracker{policy: p}
}

func (t *RestartTracker) Failed() bool {
	return t.failed
}

func (t *RestartTracker) AttemptCount() int {
	return len(t.attempts)
}

func (t *RestartTracker) NextAllowedAt() time.Time {
	return t.nextAt
}

func (t *RestartTracker) prune(now time.Time) {
	cut := now.Add(-t.policy.Window)
	kept := t.attempts[:0]
	for _, ts := range t.attempts {
		if ts.After(cut) {
			kept = append(kept, ts)
		}
	}
	t.attempts = kept
}

// CanAttempt reports whether an automatic restart may run at now.
func (t *RestartTracker) CanAttempt(now time.Time) bool {
	if t.failed {
		return false
	}
	if !t.nextAt.IsZero() && now.Before(t.nextAt) {
		return false
	}
	return true
}

// RemainingBackoff returns time left before the next allowed attempt.
func (t *RestartTracker) RemainingBackoff(now time.Time) time.Duration {
	if t.nextAt.IsZero() || !now.Before(t.nextAt) {
		return 0
	}
	return t.nextAt.Sub(now)
}

// RecordSuccess clears failure streak after stable Ready (ResetAfter).
func (t *RestartTracker) RecordSuccess(now time.Time) {
	if t.lastOK.IsZero() {
		t.lastOK = now
		return
	}
	if now.Sub(t.lastOK) >= t.policy.ResetAfter {
		t.attempts = nil
		t.attemptN = 0
		t.nextAt = time.Time{}
		t.failed = false
	}
	t.lastOK = now
}

// MarkReady notes an immediate healthy observation (resets streak after ResetAfter via RecordSuccess).
func (t *RestartTracker) MarkReady(now time.Time) {
	t.RecordSuccess(now)
	// Immediate recovery from Ready clears backoff gate.
	t.nextAt = time.Time{}
	t.attemptN = 0
	t.failed = false
	t.attempts = nil
}

// RecordFailure registers a failed recovery attempt and returns backoff and whether Failed latched.
func (t *RestartTracker) RecordFailure(now time.Time) (backoff time.Duration, latchedFailed bool) {
	t.prune(now)
	t.attempts = append(t.attempts, now)
	t.attemptN++
	backoff = t.policy.InitialBackoff << (t.attemptN - 1)
	if backoff > t.policy.MaxBackoff {
		backoff = t.policy.MaxBackoff
	}
	if backoff < t.policy.InitialBackoff {
		backoff = t.policy.InitialBackoff
	}
	t.nextAt = now.Add(backoff)
	if len(t.attempts) >= t.policy.MaxRestarts {
		t.failed = true
		latchedFailed = true
	}
	return backoff, latchedFailed
}

// ClearFailed is operator-only recovery (pause/resume or explicit admin action).
func (t *RestartTracker) ClearFailed() {
	t.failed = false
	t.attempts = nil
	t.attemptN = 0
	t.nextAt = time.Time{}
}

// Snapshot is JSON-friendly status for /api/status.
type RestartTrackerSnapshot struct {
	Failed           bool   `json:"failed"`
	AttemptsInWindow int    `json:"attemptsInWindow"`
	MaxRestarts      int    `json:"maxRestarts"`
	BackoffMS        int64  `json:"backoffMsRemaining"`
	NextAllowedAt    string `json:"nextAllowedAt,omitempty"`
}
