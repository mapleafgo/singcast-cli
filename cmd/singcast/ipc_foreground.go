package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mapleafgo/singcast/core"
	"github.com/mapleafgo/singcast/ipc"
)

func runIpcForeground(ctx context.Context, homeDir string) error {
	svc := core.NewService()
	opts, _ := json.Marshal(core.InitOptions{HomeDir: homeDir})
	if err := svc.Init(string(opts)); err != nil {
		return fmt.Errorf("init: %w", err)
	}

	srv := ipc.NewServer(svc, ipc.IpcPath())

	sigCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-ch
		slog.Info("received signal", "signal", sig)
		cancel()
	}()

	return srv.Run(sigCtx)
}
