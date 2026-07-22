package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type cycleResult struct {
	Desktop     string `json:"desktop"`
	Backend     string `json:"backend"`
	BackendPID  uint32 `json:"backendPid,omitempty"`
	BackendPort int    `json:"backendPort,omitempty"`
}

// ServiceStatus is the per-service snapshot exposed on /api/status.
type ServiceStatus struct {
	State  string        `json:"state"`
	PID    uint32        `json:"pid,omitempty"`
	Port   int           `json:"port,omitempty"`
	Health BackendHealth `json:"health,omitempty"`
}

type WatchdogState struct {
	UpdatedAt               string                    `json:"updatedAt"`
	WatchdogPID             int                       `json:"watchdogPid"`
	Paused                  bool                      `json:"paused"`
	Result                  cycleResult               `json:"result"`
	ConsecutiveBackendFails int                       `json:"consecutiveBackendFails"`
	Desktop                 ServiceStatus             `json:"desktopService"`
	Backend                 ServiceStatus             `json:"backendService"`
	Restart                 RestartTrackerSnapshot    `json:"restart"`
	Leases                  map[string]LeaseSnapshot  `json:"leases,omitempty"`
	RecentAnomalies         []AnomalySnapshot         `json:"recentAnomalies,omitempty"`
	IPCPipe                 string                    `json:"ipcPipe,omitempty"`
	ReportOnlyContract      bool                      `json:"reportOnlyContract"`
	SoleRestartAuthority    bool                      `json:"soleRestartAuthority"`
	PackagedExe             string                    `json:"packagedExe,omitempty"`
	ListenAddr              string                    `json:"listenAddr,omitempty"`
	TsnetHostname           string                    `json:"tsnetHostname,omitempty"`
	TsnetEnabled            bool                      `json:"tsnetEnabled"`
}

type Watchdog struct {
	cfg    Config
	logger *Logger
	back   *BackendManager

	mu            sync.RWMutex
	paused        bool
	failCount     int // consecutive cycles with desktop-up/backend-down (Desktop last-resort gate)
	desktopState  ServiceState
	backendState  ServiceState
	restart       *RestartTracker
	heartbeats    *HeartbeatRegistry
	anomalies     *AnomalyRegistry
	nonces        *NonceCache
	lastHealth    BackendHealth
	lastState     WatchdogState
	nowFn         func() time.Time
}

func NewWatchdog(cfg Config, logger *Logger) *Watchdog {
	policy := cfg.RestartPolicy
	if policy.MaxRestarts == 0 {
		policy = DefaultRestartPolicy()
	}
	hbTimeout := cfg.HeartbeatTimeout
	if hbTimeout <= 0 {
		hbTimeout = 45 * time.Second
	}
	nowFn := time.Now
	pipeName := cfg.IPCPipeName
	if pipeName == "" {
		pipeName = DefaultIPCPipeName
	}
	return &Watchdog{
		cfg:          cfg,
		logger:       logger,
		back:         NewBackendManager(cfg, logger),
		desktopState: StateUnknown,
		backendState: StateUnknown,
		restart:      NewRestartTracker(policy),
		heartbeats:   NewHeartbeatRegistry(hbTimeout, nowFn),
		anomalies:    NewAnomalyRegistry(cfg.AnomalyMergeWindow, nowFn),
		nonces:       NewNonceCache(10*time.Minute, nowFn),
		nowFn:        nowFn,
		lastState: WatchdogState{
			WatchdogPID:          os.Getpid(),
			PackagedExe:          cfg.PackagedExe,
			ListenAddr:           cfg.ListenAddr,
			TsnetHostname:        cfg.TsnetHostname,
			TsnetEnabled:         cfg.EnableTsnet && cfg.TsAuthKey != "",
			SoleRestartAuthority: true,
			ReportOnlyContract:   true,
			IPCPipe:              pipeName,
			Desktop:              ServiceStatus{State: StateUnknown.String()},
			Backend:              ServiceStatus{State: StateUnknown.String()},
			Restart: RestartTrackerSnapshot{
				MaxRestarts: policy.MaxRestarts,
			},
			Leases: map[string]LeaseSnapshot{
				"hermes-backend": {Epoch: 1},
				"hermes-desktop": {Epoch: 1},
			},
		},
	}
}

func (w *Watchdog) now() time.Time {
	if w.nowFn != nil {
		return w.nowFn()
	}
	return time.Now()
}

func (w *Watchdog) setDesktopState(to ServiceState) {
	from := w.desktopState
	if next, err := Transition(from, to); err == nil {
		w.desktopState = next
	} else {
		// Observed reality may jump (e.g. process vanished); force-align.
		w.logger.Infof("desktop state force %s → %s (%v)", from, to, err)
		w.desktopState = to
	}
}

func (w *Watchdog) setBackendState(to ServiceState) {
	from := w.backendState
	if next, err := Transition(from, to); err == nil {
		w.backendState = next
	} else {
		w.logger.Infof("backend state force %s → %s (%v)", from, to, err)
		w.backendState = to
	}
}

func (w *Watchdog) PrewarmBackend() {
	if !w.cfg.PrewarmBackend {
		return
	}
	w.setBackendState(StateStarting)
	if _, err := w.back.EnsureHealthy(); err != nil {
		w.logger.Infof("prewarm backend: %v", err)
		w.setBackendState(StateUnresponsive)
		return
	}
	w.setBackendState(StateReady)
}

func (w *Watchdog) findAnyHealthyBackend() *backendInfo {
	if child := findHealthyDesktopBackend(); child != nil {
		return child
	}
	if managed := w.back.currentHealthy(); managed != nil {
		return managed
	}
	return loadManifestBackend(w.cfg)
}

func (w *Watchdog) State() WatchdogState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	st := w.lastState
	st.WatchdogPID = os.Getpid()
	st.Paused = w.paused
	st.Desktop = ServiceStatus{State: w.desktopState.String()}
	st.Backend = ServiceStatus{
		State:  w.backendState.String(),
		PID:    st.Backend.PID,
		Port:   st.Backend.Port,
		Health: w.lastHealth,
	}
	st.Restart = w.restartSnapshot(w.now())
	st.SoleRestartAuthority = true
	st.ReportOnlyContract = true
	st.IPCPipe = w.cfg.IPCPipeName
	if st.IPCPipe == "" {
		st.IPCPipe = DefaultIPCPipeName
	}
	st.ConsecutiveBackendFails = w.failCount
	if w.heartbeats != nil {
		st.Leases = w.heartbeats.AllSnapshots()
	}
	if w.anomalies != nil {
		st.RecentAnomalies = w.anomalies.Recent()
	}
	return st
}

// IngestHeartbeat accepts a validated heartbeat envelope (T10/T11).
func (w *Watchdog) IngestHeartbeat(env HeartbeatEnvelope) error {
	if w.heartbeats == nil {
		return fmt.Errorf("heartbeat registry unavailable")
	}
	return w.heartbeats.Ingest(env)
}

func (w *Watchdog) backendReady(info *backendInfo) bool {
	if info != nil && info.Health.Ready {
		return true
	}
	if w.heartbeats != nil && w.heartbeats.FreshReady("hermes-backend") {
		return true
	}
	return false
}

func (w *Watchdog) backendLive(info *backendInfo) bool {
	if info != nil && info.Health.Live {
		return true
	}
	if w.heartbeats != nil && w.heartbeats.FreshLive("hermes-backend") {
		return true
	}
	return false
}

func (w *Watchdog) SetPaused(v bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.paused = v
	w.lastState.Paused = v
	if !v {
		// Operator resume clears Failed latch (manual recovery path).
		w.restart.ClearFailed()
		if w.backendState == StateFailed {
			w.backendState = StateStarting
		}
		if w.desktopState == StateFailed {
			w.desktopState = StateStarting
		}
	}
}

func (w *Watchdog) IsPaused() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.paused
}

func (w *Watchdog) restartSnapshot(now time.Time) RestartTrackerSnapshot {
	snap := RestartTrackerSnapshot{
		Failed:           w.restart.Failed(),
		AttemptsInWindow: w.restart.AttemptCount(),
		MaxRestarts:      w.restart.policy.MaxRestarts,
		BackoffMS:        w.restart.RemainingBackoff(now).Milliseconds(),
	}
	if !w.restart.NextAllowedAt().IsZero() {
		snap.NextAllowedAt = w.restart.NextAllowedAt().UTC().Format(time.RFC3339Nano)
	}
	return snap
}

func (w *Watchdog) saveState(result cycleResult, backend *backendInfo) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	var bPID uint32
	var bPort int
	var health BackendHealth
	if backend != nil {
		bPID = backend.PID
		bPort = backend.Port
		health = backend.Health
		if bPort > 0 {
			deepEvery := w.cfg.DeepHealthInterval
			if deepEvery <= 0 {
				deepEvery = 5 * time.Minute
			}
			health = probeBackendHealth(bPort, true, deepEvery, now)
		}
	}
	w.lastHealth = health
	leases := map[string]LeaseSnapshot{}
	if w.heartbeats != nil {
		leases = w.heartbeats.AllSnapshots()
	}
	pipeName := w.cfg.IPCPipeName
	if pipeName == "" {
		pipeName = DefaultIPCPipeName
	}
	var anomalies []AnomalySnapshot
	if w.anomalies != nil {
		anomalies = w.anomalies.Recent()
	}
	w.lastState = WatchdogState{
		UpdatedAt:               now.UTC().Format(time.RFC3339),
		WatchdogPID:             os.Getpid(),
		Paused:                  w.paused,
		Result:                  result,
		ConsecutiveBackendFails: w.failCount,
		Desktop:                 ServiceStatus{State: w.desktopState.String()},
		Backend: ServiceStatus{
			State:  w.backendState.String(),
			PID:    bPID,
			Port:   bPort,
			Health: health,
		},
		Restart:              w.restartSnapshot(now),
		Leases:               leases,
		RecentAnomalies:      anomalies,
		IPCPipe:              pipeName,
		ReportOnlyContract:   true,
		SoleRestartAuthority: true,
		PackagedExe:          w.cfg.PackagedExe,
		ListenAddr:           w.cfg.ListenAddr,
		TsnetHostname:        w.cfg.TsnetHostname,
		TsnetEnabled:         w.cfg.EnableTsnet && w.cfg.TsAuthKey != "",
	}
	raw, err := json.MarshalIndent(w.lastState, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(w.cfg.StatePath, raw, 0o644)
}

func (w *Watchdog) RunCycle() cycleResult {
	if w.IsPaused() {
		res := cycleResult{Desktop: "paused", Backend: "paused"}
		w.saveState(res, nil)
		return res
	}

	now := w.now()
	desktop, derr := getDesktopProcesses()
	desktopUp := derr == nil && len(desktop) > 0

	if !desktopUp {
		w.setDesktopState(StateStopped)
	} else {
		// ADR: Unknown/Stopped → Starting → Ready (never Unknown→Ready).
		if w.desktopState == StateUnknown || w.desktopState == StateStopped {
			w.setDesktopState(StateStarting)
		}
		if w.desktopState == StateStarting || w.desktopState == StateDegraded {
			w.setDesktopState(StateReady)
		}
	}

	// Auto-recovery halted: sole authority stays idle until operator resume.
	if w.restart.Failed() {
		w.setBackendState(StateFailed)
		w.logger.Infof("backend recovery FAILED — auto-restart suppressed (resume to clear)")
		res := cycleResult{Desktop: map[bool]string{true: "up", false: "down"}[desktopUp], Backend: "failed"}
		w.saveState(res, nil)
		return res
	}

	// Only ensure/spawn when nothing Ready is visible — avoid kill/respawn storms.
	if w.cfg.PrewarmBackend && w.restart.CanAttempt(now) {
		probe := w.findAnyHealthyBackend()
		if probe != nil && probe.Port > 0 && probe.Health.Source == "" {
			probe.Health = probeBackendHealth(probe.Port, false, 0, now)
		}
		if probe == nil || !w.backendReady(probe) {
			if _, err := w.back.EnsureHealthy(); err != nil {
				w.logger.Infof("ensure managed backend: %v", err)
			}
		}
	} else if !w.restart.CanAttempt(now) {
		remain := w.restart.RemainingBackoff(now)
		w.setBackendState(StateBackoff)
		w.logger.Infof("backend backoff remaining=%s — skip EnsureHealthy", remain.Round(time.Millisecond))
	}

	backend := w.findAnyHealthyBackend()
	if backend != nil && backend.Port > 0 && backend.Health.Source == "" {
		backend.Health = probeBackendHealth(backend.Port, false, 0, now)
	}

	if !desktopUp {
		var skipPID uint32
		if managed := w.back.currentHealthy(); managed != nil {
			skipPID = managed.PID
		}
		stopOrphanDesktopBackends(w.logger, w.cfg, skipPID)
		w.logger.Infof("Desktop DOWN — relaunch (watchdog sole authority)")
		w.setDesktopState(StateStarting)
		w.logger.EmitEvent(w.cfg.EventsPath, RestartEvent{
			Event:     "service_restart",
			Service:   "desktop",
			Reason:    "desktop_missing",
			FromState: StateStopped.String(),
			ToState:   StateStarting.String(),
			Command:   string(CommandStartDesktop),
		})
		w.runAllowlistedCommand(CommandStartDesktop, "desktop_missing")
		w.mu.Lock()
		w.failCount = 0
		w.mu.Unlock()
		res := cycleResult{Desktop: "relaunched", Backend: "pending"}
		w.saveState(res, backend)
		return res
	}

	// Live-only (no Ready, no fresh ready heartbeat) → Degraded, not restart storm (T03).
	if backend != nil && w.backendLive(backend) && !w.backendReady(backend) {
		w.setBackendState(StateDegraded)
		w.logger.Infof("backend LIVE but not READY (source=%s) — degraded, skip restart", backend.Health.Source)
		res := cycleResult{
			Desktop:     "up",
			Backend:     "degraded",
			BackendPID:  backend.PID,
			BackendPort: backend.Port,
		}
		w.saveState(res, backend)
		return res
	}

	if backend == nil || !w.backendReady(backend) {
		w.setBackendState(StateUnresponsive)
		if w.anomalies != nil && w.anomalies.HasMergedDual() {
			w.logger.Infof("T12 dual anomaly active — single sole-authority recovery (no peer self-restart)")
		}
		if !w.restart.CanAttempt(now) {
			w.setBackendState(StateBackoff)
			res := cycleResult{Desktop: "up", Backend: "backoff"}
			w.saveState(res, nil)
			return res
		}
		w.logger.Infof("Desktop UP but backend DOWN — starting managed serve (sole authority)")
		w.setBackendState(StateStarting)
		ok := w.runAllowlistedCommand(CommandStartBackend, "backend_missing")
		backend = w.findAnyHealthyBackend()
		if backend != nil && backend.Port > 0 {
			backend.Health = probeBackendHealth(backend.Port, false, 0, now)
		}
		if !ok || backend == nil || !w.backendReady(backend) {
			backoff, latched := w.restart.RecordFailure(now)
			w.mu.Lock()
			w.failCount++
			fails := w.failCount
			w.mu.Unlock()
			w.logger.EmitEvent(w.cfg.EventsPath, RestartEvent{
				Event:     "service_restart",
				Service:   "backend",
				Reason:    "recovery_failed",
				Attempt:   w.restart.AttemptCount(),
				BackoffMS: backoff.Milliseconds(),
				FromState: StateUnresponsive.String(),
				ToState:   map[bool]string{true: StateFailed.String(), false: StateBackoff.String()}[latched],
				Command:   string(CommandStartBackend),
			})
			if latched {
				w.setBackendState(StateFailed)
				w.logger.Infof("backend crash-loop latched Failed after %d attempts", w.restart.AttemptCount())
				res := cycleResult{Desktop: "up", Backend: "failed"}
				w.saveState(res, nil)
				return res
			}
			w.setBackendState(StateBackoff)
			w.logger.Infof("Desktop UP but backend still DOWN (fail=%d/%d backoff=%s)", fails, w.cfg.FailThreshold, backoff)
			// Desktop restart is last resort AFTER backend recovery failed enough times.
			if fails >= w.cfg.FailThreshold {
				w.logger.Infof("backend recovery exhausted — Desktop last-resort restart")
				w.setDesktopState(StateStopping)
				w.logger.EmitEvent(w.cfg.EventsPath, RestartEvent{
					Event:     "service_restart",
					Service:   "desktop",
					Reason:    "backend_recovery_exhausted",
					Attempt:   fails,
					FromState: StateReady.String(),
					ToState:   StateStarting.String(),
					Command:   string(CommandWarmRestart),
				})
				w.runAllowlistedCommand(CommandWarmRestart, "backend_recovery_exhausted")
				w.mu.Lock()
				w.failCount = 0
				w.mu.Unlock()
				w.setDesktopState(StateStarting)
				res := cycleResult{Desktop: "restarted", Backend: "respawning"}
				w.saveState(res, nil)
				return res
			}
			res := cycleResult{Desktop: "up", Backend: "down"}
			w.saveState(res, nil)
			return res
		}
	}

	w.mu.Lock()
	w.failCount = 0
	w.mu.Unlock()
	w.restart.MarkReady(now)
	// ADR transitions: Degraded→Ready directly; Backoff/Unresponsive/Stopped via Starting.
	switch w.backendState {
	case StateDegraded:
		w.setBackendState(StateReady)
	case StateBackoff, StateUnresponsive, StateStopped:
		w.setBackendState(StateStarting)
		w.setBackendState(StateReady)
	default:
		if w.backendState != StateReady && w.backendState != StateStarting {
			w.setBackendState(StateStarting)
		}
		if w.backendState == StateStarting {
			w.setBackendState(StateReady)
		} else if w.backendState != StateReady {
			w.setBackendState(StateReady)
		}
	}
	if w.desktopState == StateStarting || w.desktopState == StateStopping {
		w.setDesktopState(StateReady)
	} else if w.desktopState != StateReady {
		w.setDesktopState(StateReady)
	}
	w.logger.Infof("OK backend=pid:%d port:%d desktop=%s backend=%s health=%s", backend.PID, backend.Port, w.desktopState, w.backendState, backend.Health.Source)
	res := cycleResult{
		Desktop:     "up",
		Backend:     "up",
		BackendPID:  backend.PID,
		BackendPort: backend.Port,
	}
	w.saveState(res, backend)
	return res
}

func (w *Watchdog) RunLoop(stop <-chan struct{}) {
	w.logger.Infof("watchdog loop interval=%ds threshold=%d maxRestarts=%d exe=%s soleAuthority=true",
		w.cfg.IntervalSec, w.cfg.FailThreshold, w.cfg.RestartPolicy.MaxRestarts, w.cfg.PackagedExe)
	for {
		w.RunCycle()
		if w.cfg.Once {
			return
		}
		select {
		case <-stop:
			return
		case <-time.After(time.Duration(w.cfg.IntervalSec) * time.Second):
		}
	}
}

type lockFile struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
	RepoRoot  string `json:"repoRoot"`
}

func acquireLock(lockPath, repoRoot string, logger *Logger) (func(), bool) {
	if fileExists(lockPath) {
		raw, err := os.ReadFile(lockPath)
		if err == nil {
			var lf lockFile
			if json.Unmarshal(raw, &lf) == nil && lf.PID > 0 {
				if processAlive(lf.PID) {
					logger.Infof("another watchdog holds %s (pid=%d) — exiting", lockPath, lf.PID)
					return nil, false
				}
			}
		}
		_ = os.Remove(lockPath)
	}
	lf := lockFile{
		PID:       os.Getpid(),
		StartedAt: time.Now().Format(time.RFC3339),
		RepoRoot:  repoRoot,
	}
	raw, _ := json.MarshalIndent(lf, "", "  ")
	if err := os.WriteFile(lockPath, raw, 0o644); err != nil {
		logger.Infof("failed to write lock: %v", err)
		return nil, false
	}
	release := func() {
		raw, err := os.ReadFile(lockPath)
		if err != nil {
			return
		}
		var existing lockFile
		if json.Unmarshal(raw, &existing) == nil && existing.PID == os.Getpid() {
			_ = os.Remove(lockPath)
		}
	}
	return release, true
}

func processAlive(pid int) bool {
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), fmt.Sprintf("%d", pid))
}
