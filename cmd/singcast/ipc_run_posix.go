//go:build !windows

package main

func runIpc(homeDir string) error {
	return runIpcForeground(homeDir)
}
