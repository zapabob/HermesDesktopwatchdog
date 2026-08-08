package main

import (
	"errors"
	"testing"
)

func TestDesktopProbeStatus(t *testing.T) {
	probeFailure := errors.New("WMI unavailable")
	cases := []struct {
		name         string
		err          error
		desktopCount int
		label        string
		known        bool
		up           bool
	}{
		{"probe failure", probeFailure, 0, "unknown", false, false},
		{"desktop absent", nil, 0, "down", true, false},
		{"desktop present", nil, 1, "up", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			label, known, up := desktopProbeStatus(tc.err, tc.desktopCount)
			if label != tc.label || known != tc.known || up != tc.up {
				t.Fatalf("desktopProbeStatus(%v, %d) = (%q, %t, %t), want (%q, %t, %t)", tc.err, tc.desktopCount, label, known, up, tc.label, tc.known, tc.up)
			}
		})
	}
}
