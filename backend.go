package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const desktopBackendManifestName = "desktop-backend.json"

// DefaultManagedBackendPort is outside reserved ops ports (9119 dashboard serve, 9120 dashboard UI, …).
const DefaultManagedBackendPort = 9118

var backendReadyRE = regexp.MustCompile(`^HERMES_(?:BACKEND|DASHBOARD)_READY port=(\d+)`)

// DesktopBackendManifest is published for packaged Desktop to connect without cold-spawning serve.
type DesktopBackendManifest struct {
	BaseURL    string `json:"baseUrl"`
	Token      string `json:"token"`
	Port       int    `json:"port"`
	PID        int    `json:"pid,omitempty"`
	HermesRoot string `json:"hermesRoot,omitempty"`
	HermesHome string `json:"hermesHome,omitempty"`
	UpdatedAt  string `json:"updatedAt"`
	Managed    bool   `json:"managed"`
}

// BackendManager supervises a watchdog-owned hermes serve for fast Desktop connect.
type BackendManager struct {
	cfg    Config
	logger *Logger

	mu    sync.Mutex
	cmd   *exec.Cmd
	pid   int
	port  int
	token string
	job   *ProcessJob // Windows Job Object for managed child tree (P5)
}

func NewBackendManager(cfg Config, logger *Logger) *BackendManager {
	return &BackendManager{cfg: cfg, logger: logger}
}

func (bm *BackendManager) ManifestPath() string {
	return filepath.Join(bm.cfg.DataDir, desktopBackendManifestName)
}

func parseReadyPortLine(line string) (int, bool) {
	m := backendReadyRE.FindStringSubmatch(strings.TrimSpace(line))
	if len(m) != 2 {
		return 0, false
	}
	var port int
	if _, err := fmt.Sscanf(m[1], "%d", &port); err != nil || port <= 0 {
		return 0, false
	}
	return port, true
}

func generateSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func resolvePythonExe(hermesRoot string) string {
	if hermesRoot == "" {
		return ""
	}
	for _, rel := range []string{".venv\\Scripts\\python.exe", "venv\\Scripts\\python.exe"} {
		candidate := filepath.Join(hermesRoot, rel)
		if fileExists(candidate) {
			return candidate
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		shared := filepath.Join(home, ".hermes", "hermes-agent", "venv", "Scripts", "python.exe")
		if fileExists(shared) {
			return shared
		}
	}
	return ""
}

func resolveWebDist(hermesRoot string) string {
	if hermesRoot == "" {
		return ""
	}
	candidate := filepath.Join(hermesRoot, "hermes_cli", "web_dist")
	if fileExists(filepath.Join(candidate, "index.html")) {
		return candidate
	}
	return candidate
}

func resolveServeWorkDir(cfg Config, python string) string {
	candidates := []string{
		strings.Trim(strings.TrimSpace(cfg.HermesRoot), `"'`),
		strings.Trim(strings.TrimSpace(cfg.HermesHome), `"'`),
	}
	if python != "" {
		// shared venv: ~/.hermes/hermes-agent/venv/Scripts/python.exe → repo-ish parent
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(python), "..", "..")))
	}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return ""
}

func buildServeCommand(cfg Config) (*exec.Cmd, string, int, error) {
	python := resolvePythonExe(cfg.HermesRoot)
	if python == "" {
		return nil, "", 0, fmt.Errorf("python not found under %s (.venv or venv)", cfg.HermesRoot)
	}
	workDir := resolveServeWorkDir(cfg, python)
	if workDir == "" {
		return nil, "", 0, fmt.Errorf("no valid workdir for hermes serve (hermes-root=%q)", cfg.HermesRoot)
	}
	token, err := generateSessionToken()
	if err != nil {
		return nil, "", 0, err
	}
	port := cfg.ManagedBackendPort
	if port <= 0 {
		port = DefaultManagedBackendPort
	}
	if isReservedOpsPort(port) {
		return nil, "", 0, fmt.Errorf("managed backend port %d is reserved for ops services", port)
	}
	webDist := resolveWebDist(workDir)
	if webDist == "" {
		webDist = resolveWebDist(cfg.HermesRoot)
	}
	// --skip-build: avoid npm/web build hangs on cold start (ported from hermes-agent watchdog-go).
	cmd := exec.Command(
		python,
		"-m", "hermes_cli.main",
		"serve",
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
		"--skip-build",
	)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"HERMES_HOME="+cfg.HermesHome,
		"HERMES_DESKTOP=1",
		"HERMES_WATCHDOG_MANAGED=1",
		"HERMES_DASHBOARD_SESSION_TOKEN="+token,
		"HERMES_WEB_DIST="+webDist,
		"HERMES_DESKTOP_HERMES_ROOT="+workDir,
		"HERMES_DESKTOP_CWD="+workDir,
		"PYTHONUTF8=1",
		"PYTHONIOENCODING=utf-8",
		"PYTHONUNBUFFERED=1",
	)
	return cmd, token, port, nil
}

func (bm *BackendManager) readManifest() (*DesktopBackendManifest, error) {
	raw, err := os.ReadFile(bm.ManifestPath())
	if err != nil {
		return nil, err
	}
	var manifest DesktopBackendManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (bm *BackendManager) writeManifest(manifest DesktopBackendManifest) error {
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(bm.ManifestPath(), raw, 0o644)
}

func (bm *BackendManager) clearManifest() {
	_ = os.Remove(bm.ManifestPath())
}

func (bm *BackendManager) currentHealthy() *backendInfo {
	bm.mu.Lock()
	pid := bm.pid
	port := bm.port
	bm.mu.Unlock()
	if port <= 0 {
		return nil
	}
	if pid > 0 && !processAlive(pid) {
		return nil
	}
	if isReservedOpsPort(port) {
		return nil
	}
	h := probeBackendHealth(port, false, 0, time.Now())
	if !h.Live {
		return nil
	}
	return &backendInfo{PID: uint32(pid), Port: port, Cmd: "watchdog-managed serve", Health: h}
}

func (bm *BackendManager) stopLocked() {
	// Prefer Job Object terminate so MCP/uvicorn grandchildren die with the tree.
	if bm.job != nil && bm.job.Active() {
		if err := bm.job.Terminate(1); err != nil && bm.logger != nil {
			bm.logger.Infof("job terminate: %v — falling back to taskkill", err)
			if bm.cmd != nil && bm.cmd.Process != nil {
				stopProcessPID(uint32(bm.cmd.Process.Pid))
			} else if bm.pid > 0 {
				stopProcessPID(uint32(bm.pid))
			}
		}
		bm.job.Close()
		bm.job = nil
	} else if bm.cmd != nil && bm.cmd.Process != nil {
		stopProcessPID(uint32(bm.cmd.Process.Pid))
	} else if bm.pid > 0 {
		stopProcessPID(uint32(bm.pid))
	}
	bm.cmd = nil
	bm.pid = 0
	bm.port = 0
	bm.token = ""
}

func (bm *BackendManager) JobSnapshot() JobObjectSnapshot {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	snap := JobObjectSnapshot{Enabled: true, BackendPID: bm.pid}
	if bm.job != nil && bm.job.Active() {
		snap.Active = true
		snap.Detail = portHoldHint(bm.port)
	} else {
		snap.Detail = "no active job (process not started or already stopped)"
	}
	return snap
}

func (bm *BackendManager) waitForReadyPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if quickBackendReady(port) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for /api/status on port %d (%s)", port, timeout)
}

// EnsureHealthy keeps (or starts) the watchdog-managed serve and publishes desktop-backend.json.
func (bm *BackendManager) EnsureHealthy() (*backendInfo, error) {
	if bm.cfg.HermesRoot == "" {
		return nil, fmt.Errorf("hermes root not configured")
	}
	if existing := bm.currentHealthy(); existing != nil && existing.Health.Ready {
		if bm.token == "" {
			if manifest, err := bm.readManifest(); err == nil && manifest.Token != "" {
				bm.token = manifest.Token
			}
		}
		// /api/status can be public while gated APIs still expect the session token.
		if bm.token != "" && testBackendAuth(existing.Port, bm.token) {
			bm.mu.Lock()
			_ = bm.publishManifestLocked(existing.Port, int(existing.PID))
			bm.mu.Unlock()
			return existing, nil
		}
		bm.logger.Infof("in-memory backend auth drift on port %d; replacing", existing.Port)
		bm.mu.Lock()
		bm.stopLocked()
		bm.mu.Unlock()
		_ = stopListenersOnPort(existing.Port, bm.logger)
	}

	bm.mu.Lock()
	if bm.cmd != nil && bm.port > 0 && processAlive(bm.pid) && quickBackendReady(bm.port) {
		port, token := bm.port, bm.token
		bm.mu.Unlock()
		if token != "" && testBackendAuth(port, token) {
			bm.mu.Lock()
			info := &backendInfo{PID: uint32(bm.pid), Port: bm.port, Cmd: "watchdog-managed serve"}
			_ = bm.publishManifestLocked(bm.port, bm.pid)
			bm.mu.Unlock()
			return info, nil
		}
		bm.mu.Lock()
	}

	bm.stopLocked()

	port := bm.cfg.ManagedBackendPort
	if port <= 0 {
		port = DefaultManagedBackendPort
	}
	if isReservedOpsPort(port) {
		bm.clearManifest()
		bm.mu.Unlock()
		return nil, fmt.Errorf("managed backend port %d is reserved", port)
	}
	if quickBackendReady(port) {
		bm.port = port
		if manifest, err := bm.readManifest(); err == nil {
			if manifest.Token != "" {
				bm.token = manifest.Token
			}
			if manifest.PID > 0 && processAlive(manifest.PID) {
				bm.pid = manifest.PID
			}
		}
		reuseToken, reusePID := bm.token, bm.pid
		bm.mu.Unlock()
		// Only reuse when token unlocks gated APIs (avoids Desktop 401 on drifted token).
		if reuseToken != "" && testBackendAuth(port, reuseToken) {
			bm.mu.Lock()
			_ = bm.publishManifestLocked(port, reusePID)
			bm.mu.Unlock()
			bm.logger.Infof("reusing healthy managed backend on port %d (auth ok)", port)
			return &backendInfo{PID: uint32(reusePID), Port: port, Cmd: "existing serve on managed port"}, nil
		}
		bm.logger.Infof("managed port %d is up but session token drifted; replacing occupant", port)
		_ = stopListenersOnPort(port, bm.logger)
		time.Sleep(1 * time.Second)
		bm.mu.Lock()
		bm.port = 0
		bm.pid = 0
		bm.token = ""
	}

	cmd, token, port, err := buildServeCommand(bm.cfg)
	if err != nil {
		bm.clearManifest()
		bm.mu.Unlock()
		return nil, err
	}
	hideWindowsProcess(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		bm.mu.Unlock()
		return nil, err
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		bm.clearManifest()
		bm.mu.Unlock()
		return nil, err
	}
	go io.Copy(io.Discard, stdout)

	// Assign to Job Object ASAP so tree kill works even if readiness fails.
	if bm.job != nil {
		bm.job.Close()
		bm.job = nil
	}
	if job, jerr := NewProcessJob(); jerr != nil {
		bm.logger.Infof("job object create failed (fallback taskkill): %v", jerr)
	} else if cmd.Process != nil {
		if aerr := job.AssignPID(cmd.Process.Pid); aerr != nil {
			bm.logger.Infof("job assign pid=%d: %v", cmd.Process.Pid, aerr)
			job.Close()
		} else {
			bm.job = job
			bm.logger.Infof("job object assigned pid=%d (%s)", cmd.Process.Pid, portHoldHint(port))
		}
	}
	// Release before readiness waits so /api/status JobSnapshot cannot block for minutes.
	bm.mu.Unlock()

	if err := bm.waitForReadyPort(port, time.Duration(bm.cfg.BackendStartTimeoutSec)*time.Second); err != nil {
		if cmd.Process != nil && !processAlive(cmd.Process.Pid) {
			bm.mu.Lock()
			bm.cmd = cmd
			if cmd.Process != nil {
				bm.pid = cmd.Process.Pid
			}
			bm.stopLocked()
			bm.clearManifest()
			bm.mu.Unlock()
			return nil, fmt.Errorf("managed backend exited before /api/status became ready")
		}
		// Child uvicorn may outlive the parent wrapper — keep waiting on the fixed port.
		if err2 := bm.waitForReadyPort(port, time.Duration(bm.cfg.BackendReadyTimeoutSec)*time.Second); err2 != nil {
			bm.mu.Lock()
			bm.cmd = cmd
			if cmd.Process != nil {
				bm.pid = cmd.Process.Pid
			}
			bm.stopLocked()
			bm.clearManifest()
			bm.mu.Unlock()
			return nil, err2
		}
	}

	startedPID := 0
	if cmd.Process != nil {
		startedPID = cmd.Process.Pid
	}

	// Refuse to publish a token that does not authenticate (squatter / drift race).
	if !testBackendAuth(port, token) {
		bm.logger.Infof("managed backend status-ready but auth failed on port %d; refusing drifted manifest", port)
		bm.mu.Lock()
		bm.cmd = cmd
		bm.pid = startedPID
		bm.port = port
		bm.token = token
		bm.stopLocked()
		bm.clearManifest()
		bm.mu.Unlock()
		_ = stopListenersOnPort(port, bm.logger)
		return nil, fmt.Errorf("managed backend on port %d failed session-token auth", port)
	}

	bm.mu.Lock()
	bm.cmd = cmd
	bm.pid = startedPID
	bm.port = port
	bm.token = token
	if err := bm.publishManifestLocked(port, bm.pid); err != nil {
		bm.logger.Infof("manifest write failed: %v", err)
	}
	jobActive := bm.job != nil && bm.job.Active()
	out := &backendInfo{PID: uint32(bm.pid), Port: port, Cmd: "watchdog-managed serve"}
	bm.mu.Unlock()

	bm.logger.Infof("managed backend ready pid=%d port=%d job=%v", out.PID, out.Port, jobActive)
	return out, nil
}

func (bm *BackendManager) publishManifestLocked(port, pid int) error {
	manifest := DesktopBackendManifest{
		BaseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		Token:      bm.token,
		Port:       port,
		PID:        pid,
		HermesRoot: bm.cfg.HermesRoot,
		HermesHome: bm.cfg.HermesHome,
		UpdatedAt:  time.Now().Format(time.RFC3339),
		Managed:    true,
	}
	return bm.writeManifest(manifest)
}

func loadManifestBackend(cfg Config) *backendInfo {
	path := filepath.Join(cfg.DataDir, desktopBackendManifestName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var manifest DesktopBackendManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil
	}
	port := manifest.Port
	if port <= 0 && manifest.BaseURL != "" {
		// Best-effort parse http://127.0.0.1:NNNN
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimPrefix(manifest.BaseURL, "http://127.0.0.1:"), "%d", &parsed); err == nil {
			port = parsed
		}
	}
	if port <= 0 || isReservedOpsPort(port) {
		return nil
	}
	if manifest.PID > 0 && !processAlive(manifest.PID) {
		return nil
	}
	if !testBackendLive(port) {
		return nil
	}
	h := probeBackendHealth(port, false, 0, time.Now())
	return &backendInfo{PID: uint32(manifest.PID), Port: port, Cmd: "manifest serve", Health: h}
}

func desktopLaunchEnv(cfg Config, manifest *DesktopBackendManifest) []string {
	env := []string{
		"HERMES_HOME=" + cfg.HermesHome,
		"HERMES_DESKTOP_HERMES_ROOT=" + cfg.HermesRoot,
		"HERMES_DESKTOP_CWD=" + cfg.HermesRoot,
	}
	webDist := resolveWebDist(cfg.HermesRoot)
	if webDist != "" {
		env = append(env, "HERMES_DESKTOP_DASHBOARD_WEB_DIST="+webDist)
	}
	if manifest != nil && manifest.BaseURL != "" && manifest.Token != "" {
		env = append(env,
			"HERMES_DESKTOP_REMOTE_URL="+manifest.BaseURL,
			"HERMES_DESKTOP_REMOTE_TOKEN="+manifest.Token,
		)
	}
	return env
}
