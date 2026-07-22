package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// IPCResult is returned to HTTP / Named Pipe clients.
type IPCResult struct {
	Accepted bool            `json:"accepted"`
	Action   string          `json:"action,omitempty"` // recorded|executed|rejected|merged
	Detail   string          `json:"detail,omitempty"`
	Anomaly  AnomalySnapshot `json:"anomaly,omitempty"`
	Lease    LeaseSnapshot   `json:"lease,omitempty"`
	Command  string          `json:"command,omitempty"`
}

// HandleIPCMessage dispatches a shared envelope (HTTP or Named Pipe).
func (w *Watchdog) HandleIPCMessage(env IPCEnvelope, authRole string) (IPCResult, error) {
	if env.ProtocolVersion != ipcProtocolVersion {
		return IPCResult{}, fmt.Errorf("unsupported protocol_version %d (want %d)", env.ProtocolVersion, ipcProtocolVersion)
	}
	mt := strings.ToLower(strings.TrimSpace(env.MessageType))
	switch mt {
	case IPCMessageHeartbeat:
		var hb HeartbeatEnvelope
		hb.ProtocolVersion = env.ProtocolVersion
		hb.MessageType = "heartbeat"
		hb.InstanceID = env.InstanceID
		hb.Epoch = env.Epoch
		hb.SentAtMonoMS = env.SentAtMonoMS
		if len(env.Payload) > 0 {
			if err := json.Unmarshal(env.Payload, &hb.Payload); err != nil {
				return IPCResult{}, fmt.Errorf("invalid heartbeat payload: %w", err)
			}
		}
		if err := w.IngestHeartbeat(hb); err != nil {
			return IPCResult{}, err
		}
		svc := hb.Payload.Service
		if svc == "" {
			svc = "hermes-backend"
		}
		return IPCResult{Accepted: true, Action: "recorded", Lease: w.heartbeats.Snapshot(svc)}, nil

	case IPCMessageAnomalyReport:
		var p AnomalyPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return IPCResult{}, fmt.Errorf("invalid anomaly payload: %w", err)
		}
		if strings.TrimSpace(p.Code) == "" {
			return IPCResult{}, fmt.Errorf("anomaly code required")
		}
		if strings.TrimSpace(p.Service) == "" {
			return IPCResult{}, fmt.Errorf("anomaly service required")
		}
		// suggest_command is advisory only — never auto-executed from reports.
		snap := w.anomalies.Ingest(p)
		action := "recorded"
		if snap.Merged {
			action = "merged"
		}
		w.logger.EmitEvent(w.cfg.EventsPath, RestartEvent{
			Event:   "anomaly_report",
			Service: snap.Service,
			Reason:  snap.Code,
			Detail:  fmt.Sprintf("severity=%s merged=%v sources=%s %s", snap.Severity, snap.Merged, snap.Sources, snap.Detail),
		})
		if snap.Merged {
			w.logger.Infof("T12 dual anomaly merged code=%s sources=%s — sole authority decides", snap.Code, snap.Sources)
		}
		if w.recovery != nil && w.recovery.ObserveAnomaly(snap.Code) {
			w.logger.Infof("T04 renderer-only anomaly code=%s — skip full Desktop restart", snap.Code)
			w.logger.EmitEvent(w.cfg.EventsPath, RestartEvent{
				Event:   "renderer_only_policy",
				Service: "desktop",
				Reason:  snap.Code,
				Detail:  rendererOnlyLimitationDetail(snap.Code),
			})
			return IPCResult{
				Accepted: true,
				Action:   action,
				Anomaly:  snap,
				Detail:   rendererOnlyLimitationDetail(snap.Code),
			}, nil
		}
		return IPCResult{Accepted: true, Action: action, Anomaly: snap, Detail: "report-only; watchdog retains restart authority"}, nil

	case IPCMessageCommandRequest:
		var p CommandRequestPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return IPCResult{}, fmt.Errorf("invalid command payload: %w", err)
		}
		return w.handleCommandRequest(env, p, authRole)

	default:
		return IPCResult{}, fmt.Errorf("unsupported message_type %q", env.MessageType)
	}
}

func (w *Watchdog) handleCommandRequest(env IPCEnvelope, p CommandRequestPayload, authRole string) (IPCResult, error) {
	cmd := CommandType(strings.TrimSpace(p.Command))
	if !cmd.Valid() {
		return IPCResult{Accepted: false, Action: "rejected", Detail: "command not in allowlist"}, fmt.Errorf("command %q not allowlisted", p.Command)
	}
	requester := normalizeRequester(p.Requester)
	if authRole == "service" || reportOnlyRequester(requester) {
		// ADR P3: Desktop/Backend are report-only — they must not kill/restart peers.
		return IPCResult{
			Accepted: false,
			Action:   "rejected",
			Detail:   "report-only adapter: use anomaly_report; only operator/admin may request allowlisted commands",
			Command:  string(cmd),
		}, fmt.Errorf("report-only requester cannot execute %s", cmd)
	}
	if authRole != "admin" && authRole != "operator" {
		return IPCResult{Accepted: false, Action: "rejected", Detail: "admin auth required for command_request"}, fmt.Errorf("unauthorized command_request")
	}
	if err := w.nonces.Accept(env.Nonce); err != nil {
		return IPCResult{Accepted: false, Action: "rejected", Detail: err.Error(), Command: string(cmd)}, err
	}
	reason := strings.TrimSpace(p.Reason)
	if reason == "" {
		reason = "ipc_command_request"
	}
	ok := w.runAllowlistedCommand(cmd, reason)
	if !ok {
		return IPCResult{Accepted: false, Action: "rejected", Detail: "command execution failed", Command: string(cmd)}, fmt.Errorf("command %s failed", cmd)
	}
	return IPCResult{Accepted: true, Action: "executed", Command: string(cmd), Detail: reason}, nil
}
