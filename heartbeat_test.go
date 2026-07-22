package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHeartbeatIngestAcceptsCurrentEpoch(t *testing.T) {
	now := time.Now()
	reg := NewHeartbeatRegistry(45*time.Second, func() time.Time { return now })
	err := reg.Ingest(HeartbeatEnvelope{
		ProtocolVersion: 1,
		MessageType:     "heartbeat",
		InstanceID:      "inst-a",
		Epoch:           1,
		Payload:         HeartbeatPayload{Service: "hermes-backend", State: "ready", PID: 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reg.FreshReady("backend") {
		t.Fatal("expected fresh ready")
	}
	snap := reg.Snapshot("hermes-backend")
	if snap.Epoch != 1 || snap.InstanceID != "inst-a" || !snap.Fresh {
		t.Fatalf("snap=%+v", snap)
	}
}

func TestHeartbeatRejectsStaleEpoch(t *testing.T) {
	reg := NewHeartbeatRegistry(45*time.Second, time.Now)
	reg.BumpEpoch("hermes-backend") // now epoch 2
	err := reg.Ingest(HeartbeatEnvelope{
		ProtocolVersion: 1,
		MessageType:     "heartbeat",
		InstanceID:      "inst-a",
		Epoch:           1,
		Payload:         HeartbeatPayload{Service: "backend", State: "ready"},
	})
	if err == nil {
		t.Fatal("expected stale epoch rejection")
	}
}

func TestHeartbeatRejectsInstanceMismatch(t *testing.T) {
	reg := NewHeartbeatRegistry(45*time.Second, time.Now)
	_ = reg.Ingest(HeartbeatEnvelope{
		ProtocolVersion: 1,
		MessageType:     "heartbeat",
		InstanceID:      "inst-a",
		Epoch:           1,
		Payload:         HeartbeatPayload{Service: "backend", State: "ready"},
	})
	err := reg.Ingest(HeartbeatEnvelope{
		ProtocolVersion: 1,
		MessageType:     "heartbeat",
		InstanceID:      "inst-b",
		Epoch:           1,
		Payload:         HeartbeatPayload{Service: "backend", State: "ready"},
	})
	if err == nil {
		t.Fatal("expected instance_id mismatch")
	}
}

func TestHeartbeatTimeoutExpiresReady(t *testing.T) {
	now := time.Now()
	reg := NewHeartbeatRegistry(100*time.Millisecond, func() time.Time { return now })
	_ = reg.Ingest(HeartbeatEnvelope{
		ProtocolVersion: 1,
		MessageType:     "heartbeat",
		InstanceID:      "inst-a",
		Epoch:           1,
		Payload:         HeartbeatPayload{Service: "backend", State: "ready"},
	})
	now = now.Add(200 * time.Millisecond)
	if reg.FreshReady("backend") || reg.FreshLive("backend") {
		t.Fatal("heartbeat should be stale after timeout")
	}
}

func TestHeartbeatDegradedStateIsLiveNotReady(t *testing.T) {
	reg := NewHeartbeatRegistry(45*time.Second, time.Now)
	_ = reg.Ingest(HeartbeatEnvelope{
		ProtocolVersion: 1,
		MessageType:     "heartbeat",
		InstanceID:      "inst-a",
		Epoch:           1,
		Payload:         HeartbeatPayload{Service: "backend", State: "degraded"},
	})
	if reg.FreshReady("backend") {
		t.Fatal("degraded must not count as Ready")
	}
	if !reg.FreshLive("backend") {
		t.Fatal("degraded should count as Live")
	}
}

func TestBumpEpochInvalidatesLease(t *testing.T) {
	reg := NewHeartbeatRegistry(45*time.Second, time.Now)
	_ = reg.Ingest(HeartbeatEnvelope{
		ProtocolVersion: 1,
		MessageType:     "heartbeat",
		InstanceID:      "inst-a",
		Epoch:           1,
		Payload:         HeartbeatPayload{Service: "backend", State: "ready"},
	})
	ep := reg.BumpEpoch("backend")
	if ep != 2 {
		t.Fatalf("epoch=%d", ep)
	}
	if reg.FreshReady("backend") {
		t.Fatal("lease cleared after bump")
	}
	err := reg.Ingest(HeartbeatEnvelope{
		ProtocolVersion: 1,
		MessageType:     "heartbeat",
		InstanceID:      "inst-new",
		Epoch:           2,
		Payload:         HeartbeatPayload{Service: "backend", State: "ready"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPHeartbeatEndpoint(t *testing.T) {
	cfg := Config{
		AdminToken:       "admin",
		HeartbeatToken:   "hb-token",
		HeartbeatTimeout: 45 * time.Second,
	}
	wd := NewWatchdog(cfg, NewLogger(t.TempDir()+"/test.log"))
	srv := NewHTTPServer(cfg, wd, func() {})

	body, _ := json.Marshal(HeartbeatEnvelope{
		ProtocolVersion: 1,
		MessageType:     "heartbeat",
		InstanceID:      "i1",
		Epoch:           1,
		Payload:         HeartbeatPayload{Service: "hermes-backend", State: "ready", PID: 7},
	})
	req := httptest.NewRequest("POST", "/api/v1/heartbeat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer hb-token")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.handleHeartbeat(w, req)
	if w.Code != 200 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if !wd.heartbeats.FreshReady("hermes-backend") {
		t.Fatal("expected accepted lease")
	}

	// stale epoch after bump via command path
	wd.heartbeats.BumpEpoch("hermes-backend")
	req2 := httptest.NewRequest("POST", "/api/v1/heartbeat", bytes.NewReader(body))
	req2.Header.Set("Authorization", "Bearer hb-token")
	w2 := httptest.NewRecorder()
	srv.handleHeartbeat(w2, req2)
	if w2.Code != 409 {
		t.Fatalf("expected 409 stale epoch, got %d", w2.Code)
	}
}

func TestHTTPHeartbeatRejectsUnauthorized(t *testing.T) {
	cfg := Config{AdminToken: "admin", HeartbeatToken: "hb", HeartbeatTimeout: time.Second}
	wd := NewWatchdog(cfg, NewLogger(t.TempDir()+"/t.log"))
	srv := NewHTTPServer(cfg, wd, func() {})
	body, _ := json.Marshal(HeartbeatEnvelope{
		ProtocolVersion: 1,
		MessageType:     "heartbeat",
		InstanceID:      "i1",
		Epoch:           1,
		Payload:         HeartbeatPayload{Service: "backend", State: "ready"},
	})
	req := httptest.NewRequest("POST", "/api/v1/heartbeat", bytes.NewReader(body))
	req.RemoteAddr = "10.0.0.2:9"
	w := httptest.NewRecorder()
	srv.handleHeartbeat(w, req)
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
