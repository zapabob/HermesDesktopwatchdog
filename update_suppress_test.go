package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateSuppressEnv(t *testing.T) {
	t.Setenv("HERMES_WATCHDOG_UPDATE_IN_PROGRESS", "1")
	g := NewUpdateSuppressGate(t.TempDir(), time.Now)
	on, src, _ := g.Active()
	if !on || src != SuppressEnv {
		t.Fatalf("on=%v src=%s", on, src)
	}
}

func TestUpdateSuppressFile(t *testing.T) {
	t.Setenv("HERMES_WATCHDOG_UPDATE_IN_PROGRESS", "")
	dir := t.TempDir()
	g := NewUpdateSuppressGate(dir, time.Now)
	if err := g.WriteLockFile("installer"); err != nil {
		t.Fatal(err)
	}
	on, src, detail := g.Active()
	if !on || src != SuppressFile {
		t.Fatalf("on=%v src=%s detail=%s", on, src, detail)
	}
	if _, err := os.Stat(filepath.Join(dir, updateLockFileName)); err != nil {
		t.Fatal(err)
	}
	_ = g.ClearLockFile()
	on, _, _ = g.Active()
	if on {
		t.Fatal("expected cleared")
	}
}

func TestUpdateSuppressAPITTL(t *testing.T) {
	t.Setenv("HERMES_WATCHDOG_UPDATE_IN_PROGRESS", "")
	now := time.Unix(1000, 0)
	g := NewUpdateSuppressGate(t.TempDir(), func() time.Time { return now })
	g.SetAPI(true, 30*time.Second)
	on, src, _ := g.Active()
	if !on || src != SuppressAPI {
		t.Fatalf("on=%v src=%s", on, src)
	}
	now = now.Add(31 * time.Second)
	on, _, _ = g.Active()
	if on {
		t.Fatal("expected TTL expiry")
	}
}

func TestRunCycleSkipsWhenUpdateSuppressed(t *testing.T) {
	t.Setenv("HERMES_WATCHDOG_UPDATE_IN_PROGRESS", "1")
	cfg := Config{
		DataDir:        t.TempDir(),
		PrewarmBackend: false,
		FailThreshold:  99,
		RestartPolicy:  DefaultRestartPolicy(),
	}
	wd := NewWatchdog(cfg, NewLogger(t.TempDir()+"/u.log"))
	_ = wd.RunCycle()
	st := wd.State()
	if !st.UpdateSuppress {
		t.Fatal("expected updateSuppress on status")
	}
	if st.UpdateSuppressInfo.Source != string(SuppressEnv) {
		t.Fatalf("source=%s", st.UpdateSuppressInfo.Source)
	}
}

func TestRendererOnlyPolicy(t *testing.T) {
	p := NewRecoveryPolicy()
	if !p.ObserveAnomaly("renderer_oom") {
		t.Fatal("expected renderer_oom")
	}
	if !p.ShouldSkipFullDesktopRestart() {
		t.Fatal("expected skip")
	}
	if p.ObserveAnomaly("backend_unhealthy") {
		t.Fatal("backend should not be renderer-only")
	}
	if p.ShouldSkipFullDesktopRestart() {
		t.Fatal("sticky should clear on non-renderer")
	}
	if !IsRendererOnlyAnomaly("renderer_crash") {
		t.Fatal("renderer_crash")
	}
	if IsRendererOnlyAnomaly("main_crash") {
		t.Fatal("main_crash is full Desktop")
	}
}
