package main

import (
	"path/filepath"
	"testing"
)

func TestCommandTypeValid(t *testing.T) {
	if !CommandStartDesktop.Valid() || !CommandWarmRestart.Valid() {
		t.Fatal("expected allowlisted commands valid")
	}
	if CommandType("rm -rf /").Valid() {
		t.Fatal("free-form command must be rejected")
	}
}

func TestStatusExposesServiceStateAndAuthority(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		AdminToken:    "x",
		ListenAddr:    "127.0.0.1:9920",
		StatePath:     filepath.Join(dir, "state.json"),
		EventsPath:    filepath.Join(dir, "events.jsonl"),
		FailThreshold: 2,
		RestartPolicy: DefaultRestartPolicy(),
	}
	wd := NewWatchdog(cfg, NewLogger(filepath.Join(dir, "test.log")))
	if !wd.State().SoleRestartAuthority {
		t.Fatal("expected soleRestartAuthority")
	}
	if wd.State().Desktop.State != StateUnknown.String() {
		t.Fatalf("desktop state=%s", wd.State().Desktop.State)
	}
}
