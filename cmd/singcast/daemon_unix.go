//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func startDaemon(homeDir, configPath, ruleSetProxy, apiAddr string) error {
	logPath := filepath.Join(homeDir, "singcast.log")
	pidPath := filepath.Join(homeDir, "singcast.pid")

	// Check for stale PID file
	if pidData, err := os.ReadFile(pidPath); err == nil {
		var pid int
		if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err == nil && pid > 0 {
			if proc, err := os.FindProcess(pid); err == nil {
				if proc.Signal(syscall.Signal(0)) == nil {
					return fmt.Errorf("daemon already running (pid %d)", pid)
				}
			}
		}
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	procAttr := &syscall.ProcAttr{
		Dir:   homeDir,
		Env:   os.Environ(),
		Files: []uintptr{os.Stdin.Fd(), logFile.Fd(), logFile.Fd()},
	}

	args := []string{"singcast", "run", "-c", configPath, "--home", homeDir}
	if ruleSetProxy != "" {
		args = append(args, "--rule-set-proxy", ruleSetProxy)
	}
	if apiAddr != "" {
		args = append(args, "--api", apiAddr)
	}

	pid, err := syscall.ForkExec(self, args, procAttr)
	if err != nil {
		return fmt.Errorf("fork daemon: %w", err)
	}

	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write pid file: %v\n", err)
	}

	fmt.Printf("singcast started as daemon (pid %d)\n", pid)
	return nil
}
