//go:build windows

package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
)

// startIPCPipe serves the P3 Named Pipe primary transport (REQ-LM-09).
// Each connection reads one JSON IPCEnvelope line and writes one IPCResult line.
func startIPCPipe(logger *Logger, cfg Config, wd *Watchdog, stop <-chan struct{}) {
	pipeName := strings.TrimSpace(cfg.IPCPipeName)
	if pipeName == "" {
		pipeName = DefaultIPCPipeName
	}
	if !cfg.EnableIPCPipe {
		logger.Infof("named pipe IPC disabled")
		return
	}

	cfgPipe := &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;BA)(A;;GA;;;SY)(A;;GRGW;;;AU)", // admins/system full; authenticated users RW
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}
	ln, err := winio.ListenPipe(pipeName, cfgPipe)
	if err != nil {
		logger.Infof("named pipe listen failed on %s: %v", pipeName, err)
		return
	}
	logger.Infof("named pipe IPC listening on %s", pipeName)

	go func() {
		<-stop
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-stop:
				return
			default:
				logger.Infof("named pipe accept: %v", err)
				return
			}
		}
		go serveIPCConn(logger, wd, conn)
	}
}

func serveIPCConn(logger *Logger, wd *Watchdog, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		logger.Infof("named pipe read: %v", err)
		return
	}
	line = []byte(strings.TrimSpace(string(line)))
	if len(line) == 0 {
		return
	}
	var env IPCEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		writePipeJSON(conn, IPCResult{Accepted: false, Action: "rejected", Detail: "invalid json: " + err.Error()})
		return
	}
	// Pipe is local-only; treat as service-level unless payload claims operator
	// (command_request still requires admin HTTP or explicit operator + we reject report-only).
	authRole := "service"
	res, err := wd.HandleIPCMessage(env, authRole)
	if err != nil && !res.Accepted {
		if res.Detail == "" {
			res.Detail = err.Error()
		}
		writePipeJSON(conn, res)
		return
	}
	if err != nil && res.Accepted {
		res.Detail = err.Error()
	}
	writePipeJSON(conn, res)
}

func writePipeJSON(w io.Writer, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = w.Write(append(raw, '\n'))
}
