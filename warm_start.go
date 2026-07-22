package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// WarmStartOutcome is the terminal result of a warm-start sequence (REQ-LM-04).
// Interrupted must never be recorded as success.
type WarmStartOutcome string

const (
	WarmStartSuccess     WarmStartOutcome = "success"
	WarmStartInterrupted WarmStartOutcome = "interrupted"
	WarmStartFailed      WarmStartOutcome = "failed"
)

// WarmStartPhase names the ADR 12-step intent (Watchdog-owned subset).
type WarmStartPhase string

const (
	PhaseIntent           WarmStartPhase = "restart_intent"
	PhaseDraining         WarmStartPhase = "draining"
	PhaseStopAccepting    WarmStartPhase = "stop_accepting"
	PhaseGraceActiveRuns  WarmStartPhase = "grace_active_runs"
	PhaseCheckpointReq    WarmStartPhase = "checkpoint_request"
	PhaseCloseChildren    WarmStartPhase = "close_children_signal"
	PhaseStopBackend      WarmStartPhase = "stop_backend"
	PhaseStartBackend     WarmStartPhase = "start_backend"
	PhaseReadiness        WarmStartPhase = "readiness"
	PhaseRestoreRouting   WarmStartPhase = "restore_session_routing"
	PhaseNotifyDesktop    WarmStartPhase = "notify_desktop_backend_ready"
	PhaseResumeTraffic    WarmStartPhase = "resume_traffic"
)

// WarmStartSnapshot is exposed on /api/status.
type WarmStartSnapshot struct {
	Active          bool             `json:"active"`
	Phase           string           `json:"phase,omitempty"`
	Outcome         WarmStartOutcome `json:"outcome,omitempty"`
	Reason          string           `json:"reason,omitempty"`
	Interrupted     bool             `json:"interrupted,omitempty"`
	ResumeTraffic   bool             `json:"resumeTraffic"`
	Draining        bool             `json:"draining,omitempty"`
	ActiveRunsAtEnd int              `json:"activeRunsAtEnd,omitempty"`
	StartedAt       string           `json:"startedAt,omitempty"`
	FinishedAt      string           `json:"finishedAt,omitempty"`
	EpochBefore     int64            `json:"epochBefore,omitempty"`
	EpochAfter      int64            `json:"epochAfter,omitempty"`
	Detail          string           `json:"detail,omitempty"`
}

// WarmStartHooks are injectable so unit tests can fake clock / Hermes ack.
type WarmStartHooks struct {
	Now              func() time.Time
	ActiveRuns       func() int
	CheckpointAcked  func() bool // Hermes sets true via future ack; default false
	StopBackend      func() error
	StartBackend     func() (pid uint32, port int, err error)
	WaitReady        func(timeout time.Duration) error
	NotifyDesktop    func(port int) error // typically republish manifest
	Sleep            func(d time.Duration)
}

// WarmStartSequencer owns the Watchdog-side warm restart state machine.
type WarmStartSequencer struct {
	mu     sync.Mutex
	cfg    Config
	logger *Logger
	hooks  WarmStartHooks
	snap   WarmStartSnapshot
}

func NewWarmStartSequencer(cfg Config, logger *Logger, hooks WarmStartHooks) *WarmStartSequencer {
	if hooks.Now == nil {
		hooks.Now = time.Now
	}
	if hooks.Sleep == nil {
		hooks.Sleep = time.Sleep
	}
	if hooks.ActiveRuns == nil {
		hooks.ActiveRuns = func() int { return 0 }
	}
	if hooks.CheckpointAcked == nil {
		hooks.CheckpointAcked = func() bool { return false }
	}
	return &WarmStartSequencer{
		cfg:    cfg,
		logger: logger,
		hooks:  hooks,
		snap: WarmStartSnapshot{
			ResumeTraffic: true,
		},
	}
}

func (s *WarmStartSequencer) Snapshot() WarmStartSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap
}

func (s *WarmStartSequencer) setPhase(phase WarmStartPhase, detail string) {
	s.snap.Phase = string(phase)
	s.snap.Detail = detail
	if s.logger != nil {
		s.logger.EmitEvent(s.cfg.EventsPath, RestartEvent{
			Event:     "warm_start_phase",
			Service:   "backend",
			Reason:    s.snap.Reason,
			WarmStart: true,
			Detail:    fmt.Sprintf("phase=%s %s", phase, detail),
			FromState: s.snap.Phase,
			ToState:   string(phase),
		})
	}
}

// Run executes the Watchdog-owned warm-start sequence.
// Hermes drain/checkpoint acks are optional; missing ack → interrupted, never success if runs remain.
func (s *WarmStartSequencer) Run(reason string, bumpEpoch func() (before, after int64)) WarmStartSnapshot {
	s.mu.Lock()
	if s.snap.Active {
		s.mu.Unlock()
		out := s.Snapshot()
		out.Detail = "warm_start already in progress"
		return out
	}
	now := s.hooks.Now()
	s.snap = WarmStartSnapshot{
		Active:        true,
		Reason:        reason,
		ResumeTraffic: false,
		Draining:      true,
		StartedAt:     now.UTC().Format(time.RFC3339Nano),
	}
	s.mu.Unlock()

	drainTO := s.cfg.WarmDrainTimeout
	if drainTO <= 0 {
		drainTO = 15 * time.Second
	}
	ckptTO := s.cfg.WarmCheckpointWait
	if ckptTO <= 0 {
		ckptTO = 5 * time.Second
	}

	interrupted := false
	var outcome WarmStartOutcome = WarmStartSuccess
	var failDetail string
	activeAtEnd := 0

	emit := func(ev RestartEvent) {
		if s.logger != nil {
			ev.WarmStart = true
			if ev.Service == "" {
				ev.Service = "backend"
			}
			if ev.Reason == "" {
				ev.Reason = reason
			}
			s.logger.EmitEvent(s.cfg.EventsPath, ev)
		}
	}

	// 1. Restart intent / bump epoch
	s.mu.Lock()
	s.setPhase(PhaseIntent, "bump epoch / lease")
	if bumpEpoch != nil {
		before, after := bumpEpoch()
		s.snap.EpochBefore = before
		s.snap.EpochAfter = after
	}
	s.mu.Unlock()
	emit(RestartEvent{Event: "warm_start_intent", Detail: "epoch bumped"})

	// 2–3. Draining / stop accepting (status flag; optional HTTP notify is Hermes-side)
	s.mu.Lock()
	s.snap.Draining = true
	s.snap.ResumeTraffic = false
	s.setPhase(PhaseDraining, "draining=true stop_accepting signaled via status")
	s.setPhase(PhaseStopAccepting, "watchdog status: resumeTraffic=false")
	s.mu.Unlock()

	// 4. Grace period for active runs
	s.mu.Lock()
	s.setPhase(PhaseGraceActiveRuns, fmt.Sprintf("drain_timeout=%s", drainTO))
	s.mu.Unlock()
	deadline := s.hooks.Now().Add(drainTO)
	for s.hooks.Now().Before(deadline) {
		if s.hooks.ActiveRuns() <= 0 {
			break
		}
		s.hooks.Sleep(50 * time.Millisecond)
	}
	activeAtEnd = s.hooks.ActiveRuns()
	if activeAtEnd > 0 {
		interrupted = true
		emit(RestartEvent{
			Event:  "warm_start_interrupted",
			Detail: fmt.Sprintf("active_runs=%d missed drain deadline", activeAtEnd),
		})
	}

	// 5. Checkpoint request (machine-readable event; no Hermes ack → proceed)
	s.mu.Lock()
	s.setPhase(PhaseCheckpointReq, "emit checkpoint_request; wait optional ack")
	s.mu.Unlock()
	emit(RestartEvent{Event: "checkpoint_request", Detail: "durable state.db checkpoint requested"})
	ckptDeadline := s.hooks.Now().Add(ckptTO)
	acked := false
	for s.hooks.Now().Before(ckptDeadline) {
		if s.hooks.CheckpointAcked() {
			acked = true
			break
		}
		s.hooks.Sleep(20 * time.Millisecond)
	}
	if !acked {
		// No Hermes cooperation yet — force path; if runs were active, stay interrupted.
		emit(RestartEvent{Event: "checkpoint_timeout", Detail: "no hermes checkpoint ack; proceed force-stop"})
		if activeAtEnd > 0 {
			interrupted = true
		}
	}

	// 6. Close MCP / children — signal only (Job Object kill happens on stop)
	s.mu.Lock()
	s.setPhase(PhaseCloseChildren, "signal close; job-object kill on stop")
	s.mu.Unlock()

	// 7. Stop backend
	s.mu.Lock()
	s.setPhase(PhaseStopBackend, "stop managed backend")
	s.mu.Unlock()
	if s.hooks.StopBackend != nil {
		if err := s.hooks.StopBackend(); err != nil {
			outcome = WarmStartFailed
			failDetail = "stop_backend: " + err.Error()
			emit(RestartEvent{Event: "warm_start_failed", Detail: failDetail})
			return s.finish(outcome, interrupted, activeAtEnd, failDetail)
		}
	}

	// 8. Start backend
	s.mu.Lock()
	s.setPhase(PhaseStartBackend, "start managed backend")
	s.mu.Unlock()
	var newPID uint32
	var port int
	if s.hooks.StartBackend != nil {
		var err error
		newPID, port, err = s.hooks.StartBackend()
		if err != nil {
			outcome = WarmStartFailed
			failDetail = "start_backend: " + err.Error()
			emit(RestartEvent{Event: "warm_start_failed", Detail: failDetail, NewPID: newPID})
			return s.finish(outcome, interrupted, activeAtEnd, failDetail)
		}
	}

	// 9. Readiness
	s.mu.Lock()
	s.setPhase(PhaseReadiness, "wait ready")
	s.mu.Unlock()
	readyTO := time.Duration(s.cfg.BackendReadyTimeoutSec) * time.Second
	if readyTO <= 0 {
		readyTO = 45 * time.Second
	}
	if s.hooks.WaitReady != nil {
		if err := s.hooks.WaitReady(readyTO); err != nil {
			outcome = WarmStartFailed
			failDetail = "readiness: " + err.Error()
			emit(RestartEvent{Event: "warm_start_failed", Detail: failDetail, NewPID: newPID})
			return s.finish(outcome, interrupted, activeAtEnd, failDetail)
		}
	}

	// 10. Restore session routing — Hermes-owned; Watchdog emits contract event only
	s.mu.Lock()
	s.setPhase(PhaseRestoreRouting, "hermes-owned; contract event only")
	s.mu.Unlock()
	emit(RestartEvent{Event: "session_routing_restore_signal", Detail: "desktop/backend should restore from durable state", NewPID: newPID})

	// 11. Notify Desktop backend-ready (manifest republish)
	s.mu.Lock()
	s.setPhase(PhaseNotifyDesktop, "desktop-backend-ready / manifest")
	s.mu.Unlock()
	if s.hooks.NotifyDesktop != nil {
		if err := s.hooks.NotifyDesktop(port); err != nil && s.logger != nil {
			s.logger.Infof("warm-start notify desktop: %v", err)
		}
	}
	emit(RestartEvent{Event: "desktop_backend_ready", Detail: fmt.Sprintf("port=%d", port), NewPID: newPID})

	// 12. Resume traffic
	s.mu.Lock()
	s.setPhase(PhaseResumeTraffic, "resumeTraffic=true")
	s.snap.Draining = false
	s.snap.ResumeTraffic = true
	s.mu.Unlock()

	if interrupted {
		outcome = WarmStartInterrupted
	} else {
		outcome = WarmStartSuccess
	}
	emit(RestartEvent{
		Event:   "warm_start_complete",
		Detail:  fmt.Sprintf("outcome=%s interrupted=%v active_runs=%d", outcome, interrupted, activeAtEnd),
		NewPID:  newPID,
		Command: string(CommandWarmRestart),
	})
	return s.finish(outcome, interrupted, activeAtEnd, failDetail)
}

func (s *WarmStartSequencer) finish(outcome WarmStartOutcome, interrupted bool, activeAtEnd int, detail string) WarmStartSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Active = false
	s.snap.Outcome = outcome
	s.snap.Interrupted = interrupted || outcome == WarmStartInterrupted
	// Invariant: interrupted never recorded as success
	if s.snap.Interrupted && s.snap.Outcome == WarmStartSuccess {
		s.snap.Outcome = WarmStartInterrupted
	}
	s.snap.ActiveRunsAtEnd = activeAtEnd
	if detail != "" {
		s.snap.Detail = detail
	}
	s.snap.FinishedAt = s.hooks.Now().UTC().Format(time.RFC3339Nano)
	if outcome != WarmStartFailed {
		s.snap.Draining = false
		s.snap.ResumeTraffic = true
	}
	return s.snap
}

// IsRendererOnlyAnomaly returns true for Desktop renderer-scoped faults (REQ-LM-07).
func IsRendererOnlyAnomaly(code string) bool {
	c := strings.ToLower(strings.TrimSpace(code))
	switch c {
	case "renderer_oom", "renderer_crash", "renderer_unresponsive", "renderer_only":
		return true
	default:
		return strings.HasPrefix(c, "renderer_")
	}
}
