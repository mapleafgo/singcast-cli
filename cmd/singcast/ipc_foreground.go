package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mapleafgo/singcast/core"
	"github.com/mapleafgo/singcast/ipc"
)

func runIpcForeground(homeDir string) error {
	svc := core.NewService()
	if err := svc.Init(fmt.Sprintf(`{"home_dir":%q}`, homeDir)); err != nil {
		return fmt.Errorf("init: %w", err)
	}

	srv := ipc.NewServer(svc)

	sigCtx, cancel := context.WithCancel(context.Background())
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
