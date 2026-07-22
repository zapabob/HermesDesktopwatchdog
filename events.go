package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// RestartEvent is a machine-readable recovery log line (ADR REQ-LM-11).
type RestartEvent struct {
	Event        string `json:"event"`
	Service      string `json:"service"`
	Reason       string `json:"reason"`
	PreviousPID  uint32 `json:"previous_pid,omitempty"`
	NewPID       uint32 `json:"new_pid,omitempty"`
	Attempt      int    `json:"attempt,omitempty"`
	BackoffMS    int64  `json:"backoff_ms,omitempty"`
	WarmStart    bool   `json:"warm_start,omitempty"`
	FromState    string `json:"from_state,omitempty"`
	ToState      string `json:"to_state,omitempty"`
	Command      string `json:"command,omitempty"`
	Detail       string `json:"detail,omitempty"`
	Timestamp    string `json:"ts"`
}

func (l *Logger) EmitEvent(eventsPath string, ev RestartEvent) {
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	l.Infof("event %s", string(raw))
	if eventsPath == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(eventsPath), 0o755)
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(raw, '\n'))
}
