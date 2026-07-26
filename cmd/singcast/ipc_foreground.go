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
	if err := svc.InitContext(ctx, string(opts)); err != nil {
		return fmt.Errorf("init: %w", err)
	}

	// 优雅关闭：sing-box 需要 Close 才会落盘 cache.db 并执行 resolvectl revert，
	// 被硬杀会让系统 DNS 继续指向已消失的代理，停服后整机断网。
	defer svc.Destroy()

	srv := ipc.NewServer(svc, ipc.IpcPath(homeDir))

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
