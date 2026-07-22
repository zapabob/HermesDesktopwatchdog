//go:build !windows

package main

func startIPCPipe(logger *Logger, cfg Config, wd *Watchdog, stop <-chan struct{}) {
	logger.Infof("named pipe IPC not supported on this OS — use HTTP /api/v1/report")
}
