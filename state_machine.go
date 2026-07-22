package main

import "fmt"

// ServiceState is the lifecycle state for Desktop or Backend (ADR REQ-LM-02).
type ServiceState int

const (
	StateUnknown ServiceState = iota
	StateStarting
	StateReady
	StateDegraded
	StateUnresponsive
	StateStopping
	StateStopped
	StateBackoff
	StateFailed
)

func (s ServiceState) String() string {
	switch s {
	case StateUnknown:
		return "unknown"
	case StateStarting:
		return "starting"
	case StateReady:
		return "ready"
	case StateDegraded:
		return "degraded"
	case StateUnresponsive:
		return "unresponsive"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	case StateBackoff:
		return "backoff"
	case StateFailed:
		return "failed"
	default:
		return fmt.Sprintf("service_state(%d)", int(s))
	}
}

// allowedTransitions encodes the ADR normative transition table.
var allowedTransitions = map[ServiceState]map[ServiceState]struct{}{
	StateUnknown: {
		StateStarting: {},
		StateStopped:  {},
	},
	StateStopped: {
		StateStarting: {},
	},
	StateStarting: {
		StateReady:        {},
		StateDegraded:     {},
		StateUnresponsive: {},
		StateStopping:     {},
		StateFailed:       {},
		StateBackoff:      {}, // probe failed before Ready
	},
	StateReady: {
		StateDegraded:     {},
		StateUnresponsive: {},
		StateStopping:     {},
		StateStopped:      {}, // observed process disappearance
	},
	StateDegraded: {
		StateReady:         {},
		StateUnresponsive:  {},
		StateStopping:      {},
	},
	StateUnresponsive: {
		StateStopping: {},
		StateBackoff:  {},
		StateFailed:   {},
		StateStarting: {}, // intentional recovery / StartBackend
	},
	StateStopping: {
		StateStopped: {},
		StateBackoff: {},
	},
	StateBackoff: {
		StateStarting: {},
		StateFailed:   {},
	},
	StateFailed: {
		// Manual / operator-only recovery (e.g. resume after pause).
		StateStarting: {},
	},
}

// CanTransition reports whether from→to is an allowed ServiceState change.
func CanTransition(from, to ServiceState) bool {
	if from == to {
		return true
	}
	next, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = next[to]
	return ok
}

// Transition returns to if the move is allowed, otherwise an error.
func Transition(from, to ServiceState) (ServiceState, error) {
	if CanTransition(from, to) {
		return to, nil
	}
	return from, fmt.Errorf("illegal service state transition %s → %s", from, to)
}

// ForceTransition applies to even when illegal (for observed reality vs policy).
// Callers should prefer Transition for intentional control-plane moves.
func ForceTransition(to ServiceState) ServiceState {
	return to
}
