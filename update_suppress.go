package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const updateLockFileName = "update.lock"

// UpdateSuppressSource identifies why auto-restart is gated (P6 / T13).
type UpdateSuppressSource string

const (
	SuppressNone UpdateSuppressSource = ""
	SuppressEnv  UpdateSuppressSource = "env"
	SuppressFile UpdateSuppressSource = "file"
	SuppressAPI  UpdateSuppressSource = "api"
)

// UpdateSuppressSnapshot is exposed on /api/status.
type UpdateSuppressSnapshot struct {
	Active    bool   `json:"active"`
	Source    string `json:"source,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// UpdateSuppressGate blocks auto StartBackend/StartDesktop/WarmRestart in RunCycle.
type UpdateSuppressGate struct {
	mu        sync.Mutex
	dataDir   string
	apiOn     bool
	apiExpire time.Time
	nowFn     func() time.Time
}

func NewUpdateSuppressGate(dataDir string, nowFn func() time.Time) *UpdateSuppressGate {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &UpdateSuppressGate{dataDir: dataDir, nowFn: nowFn}
}

func (g *UpdateSuppressGate) lockPath() string {
	if g.dataDir == "" {
		return ""
	}
	return filepath.Join(g.dataDir, updateLockFileName)
}

// SetAPI enables or clears the admin API suppress flag with optional TTL.
func (g *UpdateSuppressGate) SetAPI(on bool, ttl time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.apiOn = on
	if !on || ttl <= 0 {
		g.apiExpire = time.Time{}
		return
	}
	g.apiExpire = g.nowFn().Add(ttl)
}

// Active returns whether auto-recovery must be suppressed.
func (g *UpdateSuppressGate) Active() (bool, UpdateSuppressSource, string) {
	if g == nil {
		return false, SuppressNone, ""
	}
	now := g.nowFn()

	g.mu.Lock()
	if g.apiOn {
		if !g.apiExpire.IsZero() && now.After(g.apiExpire) {
			g.apiOn = false
			g.apiExpire = time.Time{}
		} else if g.apiOn {
			exp := ""
			if !g.apiExpire.IsZero() {
				exp = g.apiExpire.UTC().Format(time.RFC3339Nano)
			}
			g.mu.Unlock()
			return true, SuppressAPI, "admin POST /api/v1/update-suppress" + ttlNote(exp)
		}
	}
	g.mu.Unlock()

	if envTruthy("HERMES_WATCHDOG_UPDATE_IN_PROGRESS") {
		return true, SuppressEnv, "HERMES_WATCHDOG_UPDATE_IN_PROGRESS"
	}

	path := g.lockPath()
	if path != "" {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return true, SuppressFile, path
		}
	}
	return false, SuppressNone, ""
}

func ttlNote(exp string) string {
	if exp == "" {
		return ""
	}
	return " expires=" + exp
}

func (g *UpdateSuppressGate) Snapshot() UpdateSuppressSnapshot {
	on, src, detail := g.Active()
	snap := UpdateSuppressSnapshot{
		Active: on,
		Source: string(src),
		Detail: detail,
	}
	g.mu.Lock()
	if g.apiOn && !g.apiExpire.IsZero() {
		snap.ExpiresAt = g.apiExpire.UTC().Format(time.RFC3339Nano)
	}
	g.mu.Unlock()
	return snap
}

// WriteLockFile creates update.lock (operator / installer helper).
func (g *UpdateSuppressGate) WriteLockFile(reason string) error {
	path := g.lockPath()
	if path == "" {
		return fmt.Errorf("data dir not configured")
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	payload := map[string]string{
		"reason":    reason,
		"createdAt": g.nowFn().UTC().Format(time.RFC3339Nano),
	}
	raw, _ := json.MarshalIndent(payload, "", "  ")
	return os.WriteFile(path, raw, 0o644)
}

// ClearLockFile removes update.lock if present.
func (g *UpdateSuppressGate) ClearLockFile() error {
	path := g.lockPath()
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// updateSuppressRequest is the admin API body.
type updateSuppressRequest struct {
	Suppress bool   `json:"suppress"`
	TTLSec   int    `json:"ttlSec,omitempty"`
	ClearFile bool  `json:"clearFile,omitempty"`
	WriteFile bool  `json:"writeFile,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func parseUpdateSuppressBody(raw []byte) (updateSuppressRequest, error) {
	var req updateSuppressRequest
	if len(raw) == 0 {
		return req, fmt.Errorf("empty body")
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return req, err
	}
	return req, nil
}

func normalizeUpdateReason(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "operator"
	}
	return s
}

func envTruthy(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
