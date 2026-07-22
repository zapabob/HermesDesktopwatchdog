package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const heartbeatProtocolVersion = 1

// HeartbeatEnvelope is the ADR IPC envelope for heartbeat messages.
type HeartbeatEnvelope struct {
	ProtocolVersion int              `json:"protocol_version"`
	MessageType     string           `json:"message_type"`
	InstanceID      string           `json:"instance_id"`
	Epoch           int64            `json:"epoch"`
	SentAtMonoMS    int64            `json:"sent_at_mono_ms"`
	Payload         HeartbeatPayload `json:"payload"`
}

// HeartbeatPayload is the service-specific heartbeat body.
type HeartbeatPayload struct {
	Service         string `json:"service"`
	PID             uint32 `json:"pid,omitempty"`
	InstanceID      string `json:"instance_id,omitempty"`
	Epoch           int64  `json:"epoch,omitempty"`
	State           string `json:"state,omitempty"`
	SessionDBReady  bool   `json:"session_db_ready,omitempty"`
	GatewayReady    bool   `json:"gateway_ready,omitempty"`
	MCPReady        bool   `json:"mcp_ready,omitempty"`
	ActiveRuns      int    `json:"active_runs,omitempty"`
	RSSBytes        int64  `json:"rss_bytes,omitempty"`
}

// LeaseSnapshot is exposed on /api/status.
type LeaseSnapshot struct {
	Epoch            int64  `json:"epoch"`
	InstanceID       string `json:"instanceId,omitempty"`
	LastHeartbeatAgo string `json:"lastHeartbeatAgo,omitempty"`
	LastState        string `json:"lastState,omitempty"`
	Fresh            bool   `json:"fresh"`
	PID              uint32 `json:"pid,omitempty"`
}

type heartbeatRecord struct {
	instanceID string
	epoch      int64
	state      string
	pid        uint32
	activeRuns int
	receivedAt time.Time // watchdog monotonic wall (time.Since safe for sleep/resume when using same clock)
}

// HeartbeatRegistry tracks leases / epochs per service (Watchdog-owned).
type HeartbeatRegistry struct {
	mu       sync.Mutex
	epochs   map[string]int64
	records  map[string]heartbeatRecord
	timeout  time.Duration
	nowFn    func() time.Time
}

func NewHeartbeatRegistry(timeout time.Duration, nowFn func() time.Time) *HeartbeatRegistry {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &HeartbeatRegistry{
		epochs:  map[string]int64{"hermes-backend": 1, "hermes-desktop": 1},
		records: make(map[string]heartbeatRecord),
		timeout: timeout,
		nowFn:   nowFn,
	}
}

func (r *HeartbeatRegistry) Epoch(service string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	service = normalizeService(service)
	if r.epochs[service] == 0 {
		r.epochs[service] = 1
	}
	return r.epochs[service]
}

// BumpEpoch invalidates stale heartbeats after restart intent (T10/T11).
func (r *HeartbeatRegistry) BumpEpoch(service string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	service = normalizeService(service)
	r.epochs[service]++
	if r.epochs[service] <= 0 {
		r.epochs[service] = 1
	}
	delete(r.records, service)
	return r.epochs[service]
}

func normalizeService(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "backend", "hermes-backend", "serve":
		return "hermes-backend"
	case "desktop", "hermes-desktop", "hermes.exe":
		return "hermes-desktop"
	default:
		if s == "" {
			return "hermes-backend"
		}
		return s
	}
}

// Ingest validates and stores a heartbeat. Rejects stale epoch / wrong instance (T10/T11).
func (r *HeartbeatRegistry) Ingest(env HeartbeatEnvelope) error {
	if env.ProtocolVersion != heartbeatProtocolVersion {
		return fmt.Errorf("unsupported protocol_version %d (want %d)", env.ProtocolVersion, heartbeatProtocolVersion)
	}
	if !strings.EqualFold(strings.TrimSpace(env.MessageType), "heartbeat") {
		return fmt.Errorf("unsupported message_type %q", env.MessageType)
	}
	service := normalizeService(env.Payload.Service)
	if service == "" {
		return fmt.Errorf("payload.service required")
	}
	instanceID := strings.TrimSpace(env.InstanceID)
	if instanceID == "" {
		instanceID = strings.TrimSpace(env.Payload.InstanceID)
	}
	if instanceID == "" {
		return fmt.Errorf("instance_id required")
	}
	epoch := env.Epoch
	if epoch == 0 {
		epoch = env.Payload.Epoch
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.epochs[service]
	if current == 0 {
		current = 1
		r.epochs[service] = current
	}
	if epoch != current {
		return fmt.Errorf("stale or future epoch %d (current %d)", epoch, current)
	}
	if prev, ok := r.records[service]; ok {
		if prev.instanceID != "" && prev.instanceID != instanceID {
			// PID reuse / reincarnation without epoch bump — reject (T11).
			return fmt.Errorf("instance_id mismatch for epoch %d (got %s want %s)", epoch, instanceID, prev.instanceID)
		}
	}
	state := strings.TrimSpace(strings.ToLower(env.Payload.State))
	if state == "" {
		state = "ready"
	}
	r.records[service] = heartbeatRecord{
		instanceID: instanceID,
		epoch:      epoch,
		state:      state,
		pid:        env.Payload.PID,
		activeRuns: env.Payload.ActiveRuns,
		receivedAt: r.nowFn(),
	}
	return nil
}

// FreshReady reports whether a recent heartbeat claims ready/degraded-ok for the service.
func (r *HeartbeatRegistry) FreshReady(service string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	service = normalizeService(service)
	rec, ok := r.records[service]
	if !ok {
		return false
	}
	if r.nowFn().Sub(rec.receivedAt) > r.timeout {
		return false
	}
	// Ready requires explicit ready claim — degraded heartbeats are live-only (T03).
	return rec.state == "ready"
}

// FreshLive is true for any non-failed fresh heartbeat (including unresponsive claims as live signal only when state is live-ish).
func (r *HeartbeatRegistry) FreshLive(service string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	service = normalizeService(service)
	rec, ok := r.records[service]
	if !ok {
		return false
	}
	return r.nowFn().Sub(rec.receivedAt) <= r.timeout
}

func (r *HeartbeatRegistry) Snapshot(service string) LeaseSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	service = normalizeService(service)
	epoch := r.epochs[service]
	if epoch == 0 {
		epoch = 1
	}
	snap := LeaseSnapshot{Epoch: epoch}
	rec, ok := r.records[service]
	if !ok {
		return snap
	}
	snap.InstanceID = rec.instanceID
	snap.LastState = rec.state
	snap.PID = rec.pid
	age := r.nowFn().Sub(rec.receivedAt)
	snap.LastHeartbeatAgo = age.Round(time.Millisecond).String()
	snap.Fresh = age <= r.timeout
	return snap
}

func (r *HeartbeatRegistry) AllSnapshots() map[string]LeaseSnapshot {
	return map[string]LeaseSnapshot{
		"hermes-backend": r.Snapshot("hermes-backend"),
		"hermes-desktop": r.Snapshot("hermes-desktop"),
	}
}

// LastActiveRuns returns the most recently reported active_runs for warm-start drain.
func (r *HeartbeatRegistry) LastActiveRuns(service string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	service = normalizeService(service)
	rec, ok := r.records[service]
	if !ok {
		return 0
	}
	return rec.activeRuns
}
