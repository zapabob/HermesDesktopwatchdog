package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

const ipcProtocolVersion = 1

// Default Named Pipe path (Windows). HTTP remains the fallback transport.
const DefaultIPCPipeName = `\\.\pipe\hermes-watchdog`

// IPC message_type values (ADR P3).
const (
	IPCMessageHeartbeat      = "heartbeat"
	IPCMessageAnomalyReport  = "anomaly_report"
	IPCMessageCommandRequest = "command_request"
)

// IPCEnvelope is the shared wire format for HTTP and Named Pipe.
type IPCEnvelope struct {
	ProtocolVersion int             `json:"protocol_version"`
	MessageType     string          `json:"message_type"`
	InstanceID      string          `json:"instance_id,omitempty"`
	Epoch           int64           `json:"epoch,omitempty"`
	SentAtMonoMS    int64           `json:"sent_at_mono_ms,omitempty"`
	Nonce           string          `json:"nonce,omitempty"`
	Payload         json.RawMessage `json:"payload"`
}

// AnomalyPayload is a report-only signal from Desktop/Backend (T12).
// Never carries an executable command line.
type AnomalyPayload struct {
	Service        string `json:"service"`
	Severity       string `json:"severity,omitempty"` // info|warn|error|critical
	Code           string `json:"code"`
	Detail         string `json:"detail,omitempty"`
	SuggestCommand string `json:"suggest_command,omitempty"` // hint only — not executed
	PID            uint32 `json:"pid,omitempty"`
}

// CommandRequestPayload requests an allowlisted CommandType.
// Free-form cmdlines are rejected. Desktop/Backend adapters must prefer anomaly_report.
type CommandRequestPayload struct {
	Command   string `json:"command"`
	Reason    string `json:"reason,omitempty"`
	Requester string `json:"requester,omitempty"` // desktop|backend|operator
}

// AnomalySnapshot is exposed on /api/status.
type AnomalySnapshot struct {
	Service   string `json:"service"`
	Severity  string `json:"severity,omitempty"`
	Code      string `json:"code"`
	Detail    string `json:"detail,omitempty"`
	Age       string `json:"age,omitempty"`
	Merged    bool   `json:"merged,omitempty"`
	Sources   string `json:"sources,omitempty"`
	Received  string `json:"receivedAt,omitempty"`
}

type anomalyRecord struct {
	service   string
	severity  string
	code      string
	detail    string
	sources   map[string]struct{}
	merged    bool
	received  time.Time
}

// AnomalyRegistry merges simultaneous Desktop+Backend reports (T12).
type AnomalyRegistry struct {
	mu          sync.Mutex
	byKey       map[string]anomalyRecord
	mergeWindow time.Duration
	nowFn       func() time.Time
	retain      time.Duration
}

func NewAnomalyRegistry(mergeWindow time.Duration, nowFn func() time.Time) *AnomalyRegistry {
	if mergeWindow <= 0 {
		mergeWindow = 5 * time.Second
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &AnomalyRegistry{
		byKey:       make(map[string]anomalyRecord),
		mergeWindow: mergeWindow,
		nowFn:       nowFn,
		retain:      10 * time.Minute,
	}
}

func anomalyKey(code, service string) string {
	code = strings.TrimSpace(strings.ToLower(code))
	service = normalizeService(service)
	return code + "|" + service
}

// Ingest stores a report. When a peer service reports the same code inside the
// merge window, marks Merged=true (T12: one Watchdog decision, not dual restart).
func (r *AnomalyRegistry) Ingest(p AnomalyPayload) AnomalySnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gcLocked()

	service := normalizeService(p.Service)
	code := strings.TrimSpace(p.Code)
	if code == "" {
		code = "unspecified"
	}
	severity := strings.ToLower(strings.TrimSpace(p.Severity))
	if severity == "" {
		severity = "warn"
	}
	now := r.nowFn()
	key := anomalyKey(code, service)
	peerKey := ""
	switch service {
	case "hermes-backend":
		peerKey = anomalyKey(code, "hermes-desktop")
	case "hermes-desktop":
		peerKey = anomalyKey(code, "hermes-backend")
	}

	rec := anomalyRecord{
		service:  service,
		severity: severity,
		code:     code,
		detail:   strings.TrimSpace(p.Detail),
		sources:  map[string]struct{}{service: {}},
		received: now,
	}

	if peerKey != "" {
		if peer, ok := r.byKey[peerKey]; ok && now.Sub(peer.received) <= r.mergeWindow {
			rec.merged = true
			rec.sources[peer.service] = struct{}{}
			peer.merged = true
			peer.sources[service] = struct{}{}
			r.byKey[peerKey] = peer
		}
	}
	if prev, ok := r.byKey[key]; ok && now.Sub(prev.received) <= r.mergeWindow {
		for s := range prev.sources {
			rec.sources[s] = struct{}{}
		}
		rec.merged = rec.merged || prev.merged || len(rec.sources) > 1
		if prev.detail != "" && rec.detail == "" {
			rec.detail = prev.detail
		}
	}
	r.byKey[key] = rec
	return r.snapshotLocked(rec)
}

func (r *AnomalyRegistry) Recent() []AnomalySnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gcLocked()
	out := make([]AnomalySnapshot, 0, len(r.byKey))
	for _, rec := range r.byKey {
		out = append(out, r.snapshotLocked(rec))
	}
	return out
}

// HasMergedDual reports whether any dual desktop+backend merge is still fresh (T12).
func (r *AnomalyRegistry) HasMergedDual() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gcLocked()
	for _, rec := range r.byKey {
		if rec.merged && len(rec.sources) >= 2 && r.nowFn().Sub(rec.received) <= r.mergeWindow*2 {
			return true
		}
	}
	return false
}

func (r *AnomalyRegistry) snapshotLocked(rec anomalyRecord) AnomalySnapshot {
	srcs := make([]string, 0, len(rec.sources))
	for s := range rec.sources {
		srcs = append(srcs, s)
	}
	return AnomalySnapshot{
		Service:  rec.service,
		Severity: rec.severity,
		Code:     rec.code,
		Detail:   rec.detail,
		Age:      r.nowFn().Sub(rec.received).Round(time.Millisecond).String(),
		Merged:   rec.merged,
		Sources:  strings.Join(srcs, ","),
		Received: rec.received.UTC().Format(time.RFC3339Nano),
	}
}

func (r *AnomalyRegistry) gcLocked() {
	now := r.nowFn()
	for k, rec := range r.byKey {
		if now.Sub(rec.received) > r.retain {
			delete(r.byKey, k)
		}
	}
}

// NonceCache rejects replayed command_request nonces (REQ-LM-10).
type NonceCache struct {
	mu     sync.Mutex
	seen   map[string]time.Time
	ttl    time.Duration
	nowFn  func() time.Time
}

func NewNonceCache(ttl time.Duration, nowFn func() time.Time) *NonceCache {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &NonceCache{seen: make(map[string]time.Time), ttl: ttl, nowFn: nowFn}
}

// Accept returns an error if nonce is empty or already seen.
func (c *NonceCache) Accept(nonce string) error {
	nonce = strings.TrimSpace(nonce)
	if nonce == "" {
		return fmt.Errorf("nonce required for command_request")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.nowFn()
	for k, t := range c.seen {
		if now.Sub(t) > c.ttl {
			delete(c.seen, k)
		}
	}
	if _, ok := c.seen[nonce]; ok {
		return fmt.Errorf("replayed nonce")
	}
	c.seen[nonce] = now
	return nil
}

func normalizeRequester(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "desktop", "hermes-desktop":
		return "desktop"
	case "backend", "hermes-backend", "serve":
		return "backend"
	case "operator", "admin", "":
		if s == "" {
			return "operator"
		}
		return "operator"
	default:
		return s
	}
}

// reportOnlyRequester is true when the caller must not execute restarts (ADR P3).
func reportOnlyRequester(requester string) bool {
	switch normalizeRequester(requester) {
	case "desktop", "backend":
		return true
	default:
		return false
	}
}
