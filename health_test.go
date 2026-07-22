package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeBackendHealthReadyImpliesLive(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := mustPortFromURL(t, srv.URL)
	h := probeBackendHealth(port, false, 0, time.Now())
	if !h.Ready || !h.Live || h.Source != "ready" {
		t.Fatalf("got %+v", h)
	}
}

func TestProbeBackendHealthLiveOnlyNotReady(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := mustPortFromURL(t, srv.URL)
	h := probeBackendHealth(port, false, 0, time.Now())
	if !h.Live || h.Ready || h.Source != "live" {
		t.Fatalf("live-only must not be Ready: %+v", h)
	}
	if testBackendStatus(port) {
		t.Fatal("testBackendStatus must be false for live-only")
	}
	if !testBackendLive(port) {
		t.Fatal("testBackendLive must be true")
	}
}

func TestProbeBackendHealthStatusFallback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := mustPortFromURL(t, srv.URL)
	h := probeBackendHealth(port, false, 0, time.Now())
	if !h.Live || !h.Ready || h.Source != "status-fallback" {
		t.Fatalf("legacy status should be ready: %+v", h)
	}
}

func TestProbeBackendHealthDeepCached(t *testing.T) {
	globalDeepCache.mu.Lock()
	globalDeepCache.ok = false
	globalDeepCache.checked = time.Time{}
	globalDeepCache.mu.Unlock()

	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/health/deep", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	port := mustPortFromURL(t, srv.URL)
	now := time.Now()
	h1 := probeBackendHealth(port, true, time.Minute, now)
	if !h1.Deep || h1.DeepCached {
		t.Fatalf("first deep: %+v", h1)
	}
	h2 := probeBackendHealth(port, true, time.Minute, now.Add(5*time.Second))
	if !h2.Deep || !h2.DeepCached {
		t.Fatalf("cached deep: %+v", h2)
	}
	if calls != 1 {
		t.Fatalf("deep called %d want 1", calls)
	}
}

func mustPortFromURL(t *testing.T, raw string) int {
	t.Helper()
	u := raw
	if len(u) < 8 {
		t.Fatalf("bad url %q", raw)
	}
	// httptest.Server.URL is http://127.0.0.1:PORT
	var port int
	for i := len(u) - 1; i >= 0; i-- {
		if u[i] == ':' {
			for _, c := range u[i+1:] {
				port = port*10 + int(c-'0')
			}
			return port
		}
	}
	t.Fatalf("no port in %q", raw)
	return 0
}
