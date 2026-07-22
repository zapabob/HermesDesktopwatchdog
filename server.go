package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

type HTTPServer struct {
	cfg      Config
	wd       *Watchdog
	shutdown func()
	mu       sync.Mutex
}

func NewHTTPServer(cfg Config, wd *Watchdog, shutdown func()) *HTTPServer {
	return &HTTPServer{cfg: cfg, wd: wd, shutdown: shutdown}
}

func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("/api/v1/report", s.handleReport)
	mux.HandleFunc("/api/v1/command", s.handleCommand)
	mux.HandleFunc("/api/v1/ipc", s.handleIPC)
	mux.HandleFunc("/api/v1/pause", s.handlePause)
	mux.HandleFunc("/api/v1/resume", s.handleResume)
	mux.HandleFunc("/api/v1/cycle", s.handleCycle)
	mux.HandleFunc("/api/v1/stop", s.handleStop)
	return mux
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *HTTPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.wd.State())
}

func (s *HTTPServer) handlePause(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.wd.SetPaused(true)
	writeJSON(w, http.StatusOK, map[string]any{"paused": true})
}

func (s *HTTPServer) handleResume(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.wd.SetPaused(false)
	writeJSON(w, http.StatusOK, map[string]any{"paused": false})
}

func (s *HTTPServer) handleCycle(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result := s.wd.RunCycle()
	writeJSON(w, http.StatusOK, result)
}

func (s *HTTPServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"stopping": "true"})
	go s.shutdown()
}

func (s *HTTPServer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireHeartbeatAuth(w, r) {
		return
	}
	var env HeartbeatEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.wd.IngestHeartbeat(env); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	service := env.Payload.Service
	if service == "" {
		service = "hermes-backend"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted": true,
		"lease":    s.wd.heartbeats.Snapshot(service),
	})
}

func (s *HTTPServer) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireHeartbeatAuth(w, r) {
		return
	}
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	var env IPCEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Bare AnomalyPayload → wrap as anomaly_report envelope.
	if strings.TrimSpace(env.MessageType) == "" && env.ProtocolVersion == 0 {
		var bare AnomalyPayload
		if err := json.Unmarshal(raw, &bare); err == nil && strings.TrimSpace(bare.Code) != "" {
			payload, _ := json.Marshal(bare)
			env = IPCEnvelope{
				ProtocolVersion: ipcProtocolVersion,
				MessageType:     IPCMessageAnomalyReport,
				Payload:         payload,
			}
		}
	}
	if strings.TrimSpace(env.MessageType) == "" {
		env.MessageType = IPCMessageAnomalyReport
	}
	if env.ProtocolVersion == 0 {
		env.ProtocolVersion = ipcProtocolVersion
	}
	res, err := s.wd.HandleIPCMessage(env, "service")
	if err != nil && !res.Accepted {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *HTTPServer) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	var env IPCEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(env.MessageType) == "" {
		env.MessageType = IPCMessageCommandRequest
	}
	if env.ProtocolVersion == 0 {
		env.ProtocolVersion = ipcProtocolVersion
	}
	res, err := s.wd.HandleIPCMessage(env, "admin")
	if err != nil && !res.Accepted {
		code := http.StatusConflict
		if res.Action == "rejected" {
			code = http.StatusForbidden
		}
		http.Error(w, err.Error(), code)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *HTTPServer) handleIPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var env IPCEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	mt := strings.ToLower(strings.TrimSpace(env.MessageType))
	authRole := "service"
	switch mt {
	case IPCMessageCommandRequest:
		if !s.requireAdmin(w, r) {
			return
		}
		authRole = "admin"
	default:
		if !s.requireHeartbeatAuth(w, r) {
			return
		}
	}
	if env.ProtocolVersion == 0 {
		env.ProtocolVersion = ipcProtocolVersion
	}
	res, err := s.wd.HandleIPCMessage(env, authRole)
	if err != nil && !res.Accepted {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// requireHeartbeatAuth allows Admin token, dedicated heartbeat token, or
// unauthenticated loopback when no tokens are configured.
func (s *HTTPServer) requireHeartbeatAuth(w http.ResponseWriter, r *http.Request) bool {
	admin := strings.TrimSpace(s.cfg.AdminToken)
	hb := strings.TrimSpace(s.cfg.HeartbeatToken)
	got := extractAdminToken(r)
	if hb != "" && got == hb {
		return true
	}
	if admin != "" && got == admin {
		return true
	}
	if admin == "" && hb == "" {
		host := r.RemoteAddr
		if i := strings.LastIndex(host, ":"); i >= 0 {
			host = host[:i]
		}
		host = strings.Trim(host, "[]")
		if host == "127.0.0.1" || host == "::1" || host == "localhost" {
			return true
		}
		http.Error(w, "heartbeat requires loopback or token", http.StatusUnauthorized)
		return false
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func (s *HTTPServer) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	token := strings.TrimSpace(s.cfg.AdminToken)
	if token == "" {
		http.Error(w, "admin token not configured — mutating API disabled", http.StatusForbidden)
		return false
	}
	got := extractAdminToken(r)
	if got == "" || got != token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func extractAdminToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	if v := strings.TrimSpace(r.Header.Get("X-Admin-Token")); v != "" {
		return v
	}
	return strings.TrimSpace(r.URL.Query().Get("admin_token"))
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
