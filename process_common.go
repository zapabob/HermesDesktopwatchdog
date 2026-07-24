package main

import "strings"

// backendInfo is a snapshot of a discovered or managed hermes serve instance.
type backendInfo struct {
	PID    uint32        `json:"pid"`
	Port   int           `json:"port"`
	Cmd    string        `json:"cmd,omitempty"`
	Health BackendHealth `json:"health,omitempty"`
}

// reservedOpsPorts are stack-owned listeners — never treat as Desktop's ephemeral hermes serve.
var reservedOpsPorts = map[int]struct{}{
	8080: {}, 8081: {}, 8646: {}, 8765: {}, 8787: {}, 9119: {}, 9120: {}, 9920: {}, 18794: {},
}

func isReservedOpsPort(port int) bool {
	_, ok := reservedOpsPorts[port]
	return ok
}

func isDesktopBackendCommandLine(cl string) bool {
	if cl == "" {
		return false
	}
	lower := strings.ToLower(cl)
	if !strings.Contains(cl, "hermes_cli.main") &&
		!strings.Contains(cl, "\\hermes.exe") &&
		!strings.Contains(cl, "Scripts\\hermes.exe") {
		return false
	}
	// Never manage gateway / harness / cron — those are stack services.
	if strings.Contains(lower, " gateway") || strings.Contains(lower, " harness") || strings.Contains(lower, " cron") {
		return false
	}
	// Explicit ops dashboard / fixed ports are not Desktop-spawned backends.
	if strings.Contains(cl, "--port 9120") || strings.Contains(cl, "--port=9120") ||
		strings.Contains(cl, "--port 8787") || strings.Contains(cl, "--port=8787") {
		return false
	}
	if strings.Contains(cl, " serve") || strings.Contains(cl, "\tserve") {
		// Prefer Desktop's ephemeral serve (--port 0). Bare "serve" still matches,
		// but find/reap skip reserved ops ports so dashboard:9120 is never claimed/killed.
		return true
	}
	if strings.Contains(cl, "dashboard") && strings.Contains(cl, "--no-open") {
		return true
	}
	return false
}

func appendUniqueInt(list []int, v int) []int {
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}
