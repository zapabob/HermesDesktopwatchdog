//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Minimal stubs so `go test` / `go build` succeed on Linux/macOS source trees.
// Full Desktop/Backend process supervision remains Windows-only.

func getDesktopProcesses() ([]win32Process, error) {
	return nil, fmt.Errorf("desktop process enumeration requires windows")
}

type win32Process struct {
	ProcessID   uint32
	Name        string
	CommandLine string
}

func getDesktopBackendCandidates() ([]win32Process, error) {
	return nil, fmt.Errorf("backend candidate enumeration requires windows")
}

func getListeningPorts(pid uint32) ([]int, error) {
	return nil, fmt.Errorf("listening-port enumeration requires windows (pid=%d)", pid)
}

func stopListenersOnPort(port int, logger *Logger) int {
	if logger != nil {
		logger.Infof("stopListenersOnPort(%d): not supported on this OS", port)
	}
	return 0
}

func findHealthyDesktopBackend() *backendInfo { return nil }

func stopProcessPID(pid uint32) {}

func stopAllDesktopProcessTrees(logger *Logger) {
	if logger != nil {
		logger.Infof("stopAllDesktopProcessTrees: not supported on this OS")
	}
}

func stopOrphanDesktopBackends(logger *Logger, cfg Config, skipPIDs ...uint32) int {
	_ = cfg
	_ = skipPIDs
	if logger != nil {
		logger.Infof("stopOrphanDesktopBackends: not supported on this OS")
	}
	return 0
}

func readLaunchManifest(cfg Config, bm *BackendManager) *DesktopBackendManifest {
	if bm != nil {
		if manifest, err := bm.readManifest(); err == nil && manifest != nil {
			return manifest
		}
	}
	path := filepath.Join(cfg.DataDir, desktopBackendManifestName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var manifest DesktopBackendManifest
	if json.Unmarshal(raw, &manifest) != nil {
		return nil
	}
	if manifest.BaseURL == "" || manifest.Token == "" {
		return nil
	}
	return &manifest
}

func startPackagedDesktop(cfg Config, logger *Logger, bm *BackendManager) bool {
	_ = bm
	if logger != nil {
		logger.Infof("packaged Desktop launch requires windows (exe=%s)", cfg.PackagedExe)
	}
	return false
}

func restartPackagedDesktop(cfg Config, logger *Logger, bm *BackendManager) bool {
	return startPackagedDesktop(cfg, logger, bm)
}
