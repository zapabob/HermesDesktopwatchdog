package main

import (
	"fmt"
	"strings"
	"sync"
)

// RecoveryPolicy decides Desktop restart granularity (REQ-LM-07 / T04).
// Renderer-only anomalies must not trigger full Desktop restart.
type RecoveryPolicy struct {
	mu sync.Mutex
	// lastRendererOnly is sticky until cleared or a non-renderer anomaly arrives.
	lastRendererOnly bool
	lastCode         string
}

func NewRecoveryPolicy() *RecoveryPolicy {
	return &RecoveryPolicy{}
}

// ObserveAnomaly updates policy from a report. Returns true when the anomaly
// is renderer-scoped (Watchdog should log/event only — no Hermes IPC recreate yet).
func (p *RecoveryPolicy) ObserveAnomaly(code string) (rendererOnly bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	rendererOnly = IsRendererOnlyAnomaly(code)
	p.lastCode = strings.TrimSpace(code)
	p.lastRendererOnly = rendererOnly
	return rendererOnly
}

// ShouldSkipFullDesktopRestart is true while the latest scoped anomaly is renderer-only.
func (p *RecoveryPolicy) ShouldSkipFullDesktopRestart() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastRendererOnly
}

// Clear resets renderer-only sticky state (e.g. after operator force restart).
func (p *RecoveryPolicy) Clear() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastRendererOnly = false
	p.lastCode = ""
}

func (p *RecoveryPolicy) Snapshot() map[string]any {
	if p == nil {
		return map[string]any{"rendererOnlySkip": false}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return map[string]any{
		"rendererOnlySkip": p.lastRendererOnly,
		"lastCode":         p.lastCode,
		"limitation":       "renderer recreate requires Hermes Desktop IPC; Watchdog logs/events only",
	}
}

func rendererOnlyLimitationDetail(code string) string {
	return fmt.Sprintf(
		"anomaly=%s treated as renderer-only: skip full Desktop restart; "+
			"Hermes-side renderer recreate IPC not available in this repo",
		code,
	)
}
