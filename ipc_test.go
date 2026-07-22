package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAnomalyMergeDualReportT12(t *testing.T) {
	now := time.Now()
	reg := NewAnomalyRegistry(5*time.Second, func() time.Time { return now })
	a := reg.Ingest(AnomalyPayload{Service: "hermes-desktop", Code: "backend_unhealthy", Severity: "error"})
	if a.Merged {
		t.Fatal("first report should not be merged yet")
	}
	now = now.Add(1 * time.Second)
	b := reg.Ingest(AnomalyPayload{Service: "hermes-backend", Code: "backend_unhealthy", Severity: "error"})
	if !b.Merged {
		t.Fatalf("peer report should merge: %+v", b)
	}
	if !reg.HasMergedDual() {
		t.Fatal("expected HasMergedDual")
	}
}

func TestReportOnlyRejectsDesktopCommand(t *testing.T) {
	cfg := Config{AdminToken: "admin", HeartbeatTimeout: time.Second, AnomalyMergeWindow: 5 * time.Second}
	wd := NewWatchdog(cfg, NewLogger(t.TempDir()+"/t.log"))
	payload, _ := json.Marshal(CommandRequestPayload{
		Command:   string(CommandStopBackend),
		Reason:    "desktop wants kill",
		Requester: "desktop",
	})
	res, err := wd.HandleIPCMessage(IPCEnvelope{
		ProtocolVersion: 1,
		MessageType:     IPCMessageCommandRequest,
		Nonce:           "n1",
		Payload:         payload,
	}, "service")
	if err == nil || res.Accepted {
		t.Fatalf("desktop must be report-only: res=%+v err=%v", res, err)
	}
	if res.Action != "rejected" {
		t.Fatalf("action=%s", res.Action)
	}
}

func TestAdminAllowlistedCommandRequiresNonce(t *testing.T) {
	cfg := Config{AdminToken: "admin", HeartbeatTimeout: time.Second}
	wd := NewWatchdog(cfg, NewLogger(t.TempDir()+"/t.log"))
	payload, _ := json.Marshal(CommandRequestPayload{
		Command:   string(CommandStopBackend),
		Requester: "operator",
	})
	_, err := wd.HandleIPCMessage(IPCEnvelope{
		ProtocolVersion: 1,
		MessageType:     IPCMessageCommandRequest,
		Payload:         payload,
	}, "admin")
	if err == nil {
		t.Fatal("nonce required")
	}
}

func TestNonceReplayRejected(t *testing.T) {
	c := NewNonceCache(time.Minute, time.Now)
	if err := c.Accept("abc"); err != nil {
		t.Fatal(err)
	}
	if err := c.Accept("abc"); err == nil {
		t.Fatal("replay must fail")
	}
}

func TestFreeFormCommandRejected(t *testing.T) {
	cfg := Config{AdminToken: "admin"}
	wd := NewWatchdog(cfg, NewLogger(t.TempDir()+"/t.log"))
	payload, _ := json.Marshal(CommandRequestPayload{Command: "rm -rf /", Requester: "operator"})
	res, err := wd.HandleIPCMessage(IPCEnvelope{
		ProtocolVersion: 1,
		MessageType:     IPCMessageCommandRequest,
		Nonce:           "n2",
		Payload:         payload,
	}, "admin")
	if err == nil || res.Accepted {
		t.Fatal("free-form must be rejected")
	}
}

func TestHTTPReportEndpoint(t *testing.T) {
	cfg := Config{HeartbeatTimeout: time.Second, AnomalyMergeWindow: 5 * time.Second}
	wd := NewWatchdog(cfg, NewLogger(t.TempDir()+"/t.log"))
	srv := NewHTTPServer(cfg, wd, func() {})
	body := []byte(`{"protocol_version":1,"message_type":"anomaly_report","payload":{"service":"hermes-desktop","code":"api_dead","severity":"warn"}}`)
	req := httptest.NewRequest("POST", "/api/v1/report", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:9"
	w := httptest.NewRecorder()
	srv.handleReport(w, req)
	if w.Code != 200 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	st := wd.State()
	if !st.ReportOnlyContract {
		t.Fatal("reportOnlyContract expected")
	}
	if len(st.RecentAnomalies) == 0 {
		t.Fatal("expected anomaly in status")
	}
}

func TestHTTPCommandRejectsWithoutAdmin(t *testing.T) {
	cfg := Config{AdminToken: "secret"}
	wd := NewWatchdog(cfg, NewLogger(t.TempDir()+"/t.log"))
	srv := NewHTTPServer(cfg, wd, func() {})
	payload, _ := json.Marshal(map[string]any{
		"protocol_version": 1,
		"message_type":     "command_request",
		"nonce":            "x",
		"payload":          map[string]string{"command": "start_backend", "requester": "operator"},
	})
	req := httptest.NewRequest("POST", "/api/v1/command", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	srv.handleCommand(w, req)
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
