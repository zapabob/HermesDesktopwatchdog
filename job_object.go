package main

import "fmt"

// JobObjectSnapshot is exposed on /api/status (P5).
type JobObjectSnapshot struct {
	Enabled    bool   `json:"enabled"`
	Active     bool   `json:"active"`
	BackendPID int    `json:"backendPid,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

func portHoldHint(port int) string {
	if port <= 0 {
		return ""
	}
	return fmt.Sprintf("if port %d stays LISTENING after job terminate, treat as port-hold; skip managed reap and wait readiness reuse", port)
}
