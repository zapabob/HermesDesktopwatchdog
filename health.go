package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// BackendHealth is the multi-level probe result (ADR REQ-LM-06).
// Ready requires /ready (or legacy /api/status fallback). Live alone is never Ready.
type BackendHealth struct {
	Live       bool   `json:"live"`
	Ready      bool   `json:"ready"`
	Deep       bool   `json:"deep,omitempty"`
	DeepCached bool   `json:"deepCached,omitempty"`
	Source     string `json:"source,omitempty"` // ready|live|status-fallback|none
}

type deepHealthCache struct {
	mu      sync.Mutex
	ok      bool
	checked time.Time
}

var globalDeepCache deepHealthCache

func httpOK(client *http.Client, url string) bool {
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// probeBackendHealth probes /live, /ready, and optionally /health/deep.
// If /ready is missing, falls back to legacy /api/status as Ready (compat).
// If only /live answers, Ready stays false (T03: process/API-live ≠ session-ready).
func probeBackendHealth(port int, wantDeep bool, deepMaxAge time.Duration, now time.Time) BackendHealth {
	if port <= 0 {
		return BackendHealth{Source: "none"}
	}
	client := &http.Client{Timeout: 3 * time.Second}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	live := httpOK(client, base+"/live")
	ready := httpOK(client, base+"/ready")
	source := "none"
	if ready {
		source = "ready"
		if !live {
			live = true // ready implies live
		}
	} else if live {
		source = "live"
	} else if httpOK(client, base+"/api/status") {
		// Legacy Hermes serve: /api/status counts as both live+ready until native routes exist.
		live = true
		ready = true
		source = "status-fallback"
	}

	h := BackendHealth{Live: live, Ready: ready, Source: source}
	if !wantDeep || !live {
		return h
	}

	globalDeepCache.mu.Lock()
	defer globalDeepCache.mu.Unlock()
	if deepMaxAge > 0 && !globalDeepCache.checked.IsZero() && now.Sub(globalDeepCache.checked) < deepMaxAge {
		h.Deep = globalDeepCache.ok
		h.DeepCached = true
		return h
	}
	ok := httpOK(client, base+"/health/deep")
	globalDeepCache.ok = ok
	globalDeepCache.checked = now
	h.Deep = ok
	h.DeepCached = false
	return h
}

// testBackendStatus reports whether the backend is Ready (or legacy status OK).
// Live-only backends return false so callers do not treat them as session-ready.
func testBackendStatus(port int) bool {
	h := probeBackendHealth(port, false, 0, time.Now())
	return h.Ready
}

// quickBackendReady is a short-timeout Ready check for startup wait loops.
// Prefers /ready, then legacy /api/status (skips /live to avoid stacked timeouts).
func quickBackendReady(port int) bool {
	if port <= 0 {
		return false
	}
	client := &http.Client{Timeout: 800 * time.Millisecond}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	if httpOK(client, base+"/ready") {
		return true
	}
	return httpOK(client, base+"/api/status")
}

// testBackendLive reports liveness only (process/event-loop answer).
func testBackendLive(port int) bool {
	h := probeBackendHealth(port, false, 0, time.Now())
	return h.Live
}

// testBackendAuth verifies the session token unlocks a gated API.
// /api/status is often public, so LISTEN+status-OK can still mean token drift.
func testBackendAuth(port int, token string) bool {
	if port <= 0 || strings.TrimSpace(token) == "" {
		return false
	}
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/sessions", port), nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Hermes-Session-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
