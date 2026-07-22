package main

// CommandType is the allowlisted control-plane action set (ADR REQ-LM-10).
// Free-form command lines from Desktop are rejected by design.
type CommandType string

const (
	CommandStartDesktop CommandType = "start_desktop"
	CommandStartBackend CommandType = "start_backend"
	CommandStopBackend  CommandType = "stop_backend"
	CommandWarmRestart  CommandType = "warm_restart"
	CommandStopDesktop  CommandType = "stop_desktop"
)

func (c CommandType) Valid() bool {
	switch c {
	case CommandStartDesktop, CommandStartBackend, CommandStopBackend, CommandWarmRestart, CommandStopDesktop:
		return true
	default:
		return false
	}
}

// runAllowlistedCommand executes a fixed recovery action. P1 wires internal
// call sites through this helper so later IPC can share the same allowlist.
func (w *Watchdog) runAllowlistedCommand(cmd CommandType, reason string) bool {
	if !cmd.Valid() {
		w.logger.Infof("rejected unknown command %q", cmd)
		return false
	}
	w.logger.EmitEvent(w.cfg.EventsPath, RestartEvent{
		Event:   "command",
		Service: "watchdog",
		Reason:  reason,
		Command: string(cmd),
	})
	switch cmd {
	case CommandStartDesktop:
		if w.heartbeats != nil {
			w.heartbeats.BumpEpoch("hermes-desktop")
		}
		return startPackagedDesktop(w.cfg, w.logger, w.back)
	case CommandStopDesktop:
		if w.heartbeats != nil {
			w.heartbeats.BumpEpoch("hermes-desktop")
		}
		stopAllDesktopProcessTrees(w.logger)
		return true
	case CommandStartBackend:
		if w.heartbeats != nil {
			w.heartbeats.BumpEpoch("hermes-backend")
		}
		_, err := w.back.EnsureHealthy()
		return err == nil
	case CommandStopBackend:
		if w.heartbeats != nil {
			w.heartbeats.BumpEpoch("hermes-backend")
		}
		w.back.mu.Lock()
		w.back.stopLocked()
		w.back.mu.Unlock()
		return true
	case CommandWarmRestart:
		if w.heartbeats != nil {
			w.heartbeats.BumpEpoch("hermes-desktop")
			w.heartbeats.BumpEpoch("hermes-backend")
		}
		// P1: warm restart is Desktop last-resort restart (full drain lands in P4).
		return restartPackagedDesktop(w.cfg, w.logger, w.back)
	default:
		return false
	}
}
