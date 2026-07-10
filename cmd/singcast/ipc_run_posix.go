//go:build !windows

package main

import "context"

func runIpc(ctx context.Context, homeDir string) error {
	return runIpcForeground(ctx, homeDir)
}
