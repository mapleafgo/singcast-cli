//go:build !windows

package main

import (
	"os"
	"syscall"
)

func shutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP}
}

func isReloadSignal(sig os.Signal) bool {
	return sig == syscall.SIGHUP
}
