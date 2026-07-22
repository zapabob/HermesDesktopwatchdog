package main

import "testing"

func TestServiceStateString(t *testing.T) {
	if StateReady.String() != "ready" {
		t.Fatalf("got %q", StateReady.String())
	}
	if StateFailed.String() != "failed" {
		t.Fatalf("got %q", StateFailed.String())
	}
}

func TestCanTransitionNormative(t *testing.T) {
	allowed := []struct{ from, to ServiceState }{
		{StateStopped, StateStarting},
		{StateStarting, StateReady},
		{StateReady, StateDegraded},
		{StateReady, StateStopped},
		{StateDegraded, StateUnresponsive},
		{StateUnresponsive, StateStopping},
		{StateUnresponsive, StateStarting},
		{StateStopping, StateBackoff},
		{StateBackoff, StateStarting},
		{StateUnresponsive, StateFailed},
		{StateFailed, StateStarting},
	}
	for _, tc := range allowed {
		if !CanTransition(tc.from, tc.to) {
			t.Fatalf("expected allow %s → %s", tc.from, tc.to)
		}
	}
}

func TestCanTransitionIllegal(t *testing.T) {
	illegal := []struct{ from, to ServiceState }{
		{StateBackoff, StateReady}, // must go via Starting
		{StateReady, StateFailed},
		{StateFailed, StateReady},
		{StateStopped, StateReady},
	}
	for _, tc := range illegal {
		if CanTransition(tc.from, tc.to) {
			t.Fatalf("expected deny %s → %s", tc.from, tc.to)
		}
		if _, err := Transition(tc.from, tc.to); err == nil {
			t.Fatalf("Transition should err for %s → %s", tc.from, tc.to)
		}
	}
}

func TestSameStateTransitionAllowed(t *testing.T) {
	if !CanTransition(StateReady, StateReady) {
		t.Fatal("identity transition must be allowed")
	}
}
