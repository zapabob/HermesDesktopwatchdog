package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseReadyPortLine(t *testing.T) {
	cases := []struct {
		line string
		want int
		ok   bool
	}{
		{"HERMES_BACKEND_READY port=43210", 43210, true},
		{"HERMES_DASHBOARD_READY port=9123", 9123, true},
		{"noise", 0, false},
		{"HERMES_BACKEND_READY port=0", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseReadyPortLine(tc.line)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("line %q => (%d,%v) want (%d,%v)", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

func TestBuildServeCommandIncludesSkipBuild(t *testing.T) {
	dir := t.TempDir()
	venvPy := filepath.Join(dir, ".venv", "Scripts")
	if err := os.MkdirAll(venvPy, 0o755); err != nil {
		t.Fatal(err)
	}
	pyPath := filepath.Join(venvPy, "python.exe")
	if err := os.WriteFile(pyPath, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		HermesRoot:         dir,
		HermesHome:         dir,
		ManagedBackendPort: DefaultManagedBackendPort,
	}
	cmd, token, port, err := buildServeCommand(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || port != DefaultManagedBackendPort {
		t.Fatalf("unexpected token/port: %q %d", token, port)
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "serve") || !strings.Contains(joined, "--skip-build") {
		t.Fatalf("expected serve --skip-build in args, got %v", cmd.Args)
	}
}

func TestBuildServeCommandReusesPreferredToken(t *testing.T) {
	dir := t.TempDir()
	venvPy := filepath.Join(dir, ".venv", "Scripts")
	if err := os.MkdirAll(venvPy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(venvPy, "python.exe"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		HermesRoot:         dir,
		HermesHome:         dir,
		ManagedBackendPort: DefaultManagedBackendPort,
	}
	cmd, token, _, err := buildServeCommand(cfg, "stable-session-token")
	if err != nil {
		t.Fatal(err)
	}
	if token != "stable-session-token" {
		t.Fatalf("expected preferred token reuse, got %q", token)
	}
	envJoined := strings.Join(cmd.Env, ";")
	if !strings.Contains(envJoined, "HERMES_DASHBOARD_SESSION_TOKEN=stable-session-token") {
		t.Fatalf("preferred token missing from serve env: %q", envJoined)
	}
}

// stopLocked must keep the session token: Desktop still holds it via
// HERMES_DESKTOP_REMOTE_TOKEN, so re-minting on every flap causes 401 drift.
func TestStopLockedKeepsSessionToken(t *testing.T) {
	dir := t.TempDir()
	bm := NewBackendManager(Config{DataDir: dir}, NewLogger(filepath.Join(dir, "t.log")))
	bm.mu.Lock()
	bm.token = "keep-me"
	bm.port = 9119
	bm.stopLocked()
	token, port := bm.token, bm.port
	bm.mu.Unlock()
	if token != "keep-me" {
		t.Fatalf("expected token retained after stopLocked, got %q", token)
	}
	if port != 0 {
		t.Fatalf("expected port cleared after stopLocked, got %d", port)
	}
}

func TestWaitManagedPortClearedEmptyPort(t *testing.T) {
	if !waitManagedPortCleared(0, time.Second, nil) {
		t.Fatal("port 0 should be treated as cleared")
	}
}

// Desktop and operator tooling also write desktop-backend.json, using the legacy
// "url" key and a unix-ms "updatedAt". Those must not make the manifest unreadable.
func TestReadManifestToleratesNumericUpdatedAtAndURLAlias(t *testing.T) {
	dir := t.TempDir()
	bm := NewBackendManager(Config{DataDir: dir}, NewLogger(filepath.Join(dir, "t.log")))
	raw := []byte(`{"url":"http://127.0.0.1:9119","token":"tok","port":9119,"updatedAt":1784904830048}`)
	if err := os.WriteFile(bm.ManifestPath(), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := bm.readManifest()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Token != "tok" || got.BaseURL != "http://127.0.0.1:9119" || got.Port != 9119 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestResolvePythonExe(t *testing.T) {
	dir := t.TempDir()
	venvPy := filepath.Join(dir, ".venv", "Scripts")
	if err := os.MkdirAll(venvPy, 0o755); err != nil {
		t.Fatal(err)
	}
	pyPath := filepath.Join(venvPy, "python.exe")
	if err := os.WriteFile(pyPath, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := resolvePythonExe(dir)
	if got != pyPath {
		t.Fatalf("expected %q got %q", pyPath, got)
	}
}

func TestDesktopLaunchEnvIncludesRemoteWhenManifest(t *testing.T) {
	cfg := Config{
		HermesRoot: `C:\repo`,
		HermesHome: `C:\Users\u\.hermes`,
	}
	manifest := &DesktopBackendManifest{
		BaseURL: "http://127.0.0.1:54321",
		Token:   "tok",
	}
	env := desktopLaunchEnv(cfg, manifest)
	joined := stringsJoinEnv(env)
	for _, want := range []string{
		"HERMES_DESKTOP_REMOTE_URL=http://127.0.0.1:54321",
		"HERMES_DESKTOP_REMOTE_TOKEN=tok",
		"HERMES_DESKTOP_HERMES_ROOT=C:\\repo",
	} {
		if !containsSubstr(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}

func TestDesktopLaunchEnvClearsRemoteWithoutManifest(t *testing.T) {
	cfg := Config{HermesRoot: `C:\repo`, HermesHome: `C:\Users\u\.hermes`}
	env := desktopLaunchEnv(cfg, nil)
	joined := stringsJoinEnv(env)
	if !containsSubstr(joined, "HERMES_DESKTOP_REMOTE_URL=") || !containsSubstr(joined, "HERMES_DESKTOP_REMOTE_TOKEN=") {
		t.Fatalf("expected explicit remote clears, got %q", joined)
	}
	if containsSubstr(joined, "HERMES_DESKTOP_REMOTE_URL=http") {
		t.Fatalf("unexpected remote URL value in %q", joined)
	}
	// Incomplete manifest (URL only) must also clear — never inject URL-without-TOKEN.
	env2 := desktopLaunchEnv(cfg, &DesktopBackendManifest{BaseURL: "http://127.0.0.1:9119", Token: ""})
	joined2 := stringsJoinEnv(env2)
	if containsSubstr(joined2, "HERMES_DESKTOP_REMOTE_URL=http://127.0.0.1:9119") {
		t.Fatalf("URL-only manifest must not inject REMOTE_URL: %q", joined2)
	}
}

func TestStripInheritedDesktopRemotes(t *testing.T) {
	base := []string{
		"PATH=C:\\Windows",
		"HERMES_DESKTOP_REMOTE_URL=http://stale:9118",
		"HERMES_DESKTOP_REMOTE_TOKEN=old",
		"HERMES_HOME=C:\\Users\\u\\.hermes",
	}
	got := stripInheritedDesktopRemotes(base)
	joined := stringsJoinEnv(got)
	if containsSubstr(joined, "HERMES_DESKTOP_REMOTE_URL=") || containsSubstr(joined, "HERMES_DESKTOP_REMOTE_TOKEN=") {
		t.Fatalf("expected remotes stripped, got %q", joined)
	}
	if !containsSubstr(joined, "PATH=C:\\Windows") || !containsSubstr(joined, "HERMES_HOME=") {
		t.Fatalf("expected other env preserved, got %q", joined)
	}
}

func stringsJoinEnv(env []string) string {
	out := ""
	for _, e := range env {
		out += e + ";"
	}
	return out
}

func containsSubstr(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestBackendManagerWriteReadManifest(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		DataDir:    dir,
		HermesRoot: dir,
		HermesHome: dir,
	}
	logger := NewLogger(filepath.Join(dir, "test.log"))
	bm := NewBackendManager(cfg, logger)
	bm.mu.Lock()
	bm.token = "abc"
	bm.mu.Unlock()
	if err := bm.publishManifestLocked(12345, 999); err != nil {
		t.Fatal(err)
	}
	bm.mu.Lock()
	bm.token = "updated"
	bm.mu.Unlock()
	if err := bm.publishManifestLocked(23456, 1000); err != nil {
		t.Fatal(err)
	}
	got, err := bm.readManifest()
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != 23456 || got.Token != "updated" || !got.Managed {
		t.Fatalf("unexpected manifest: %+v", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("temporary manifest file left behind: %s", entry.Name())
		}
	}
}
