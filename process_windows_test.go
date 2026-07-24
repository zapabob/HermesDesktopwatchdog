//go:build windows

package main

import "testing"

func TestListeningPIDsOnPortInvalid(t *testing.T) {
	if got := listeningPIDsOnPort(0); got != nil {
		t.Fatalf("expected nil for port 0, got %v", got)
	}
	if got := listeningPIDsOnPort(-1); got != nil {
		t.Fatalf("expected nil for negative port, got %v", got)
	}
}

func TestStopListenersOnPortInvalid(t *testing.T) {
	if n := stopListenersOnPort(0, nil); n != 0 {
		t.Fatalf("expected 0 kills for port 0, got %d", n)
	}
}
