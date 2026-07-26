//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"golang.org/x/sys/windows/svc"

	"github.com/mapleafgo/singcast/core"
	"github.com/mapleafgo/singcast/ipc"
)

type windowsServiceHandler struct {
	homeDir string
}

func (w *windowsServiceHandler) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	s <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svcInst := core.NewService()
	opts, _ := json.Marshal(core.InitOptions{HomeDir: w.homeDir})
	if err := svcInst.InitContext(ctx, string(opts)); err != nil {
		slog.Error("init service", "error", err)
		s <- svc.Status{State: svc.Stopped}
		return true, 1
	}
	// 优雅关闭：见 runIpcForeground 的同一处说明。
	defer svcInst.Destroy()

	srv := ipc.NewServer(svcInst, ipc.IpcPath(w.homeDir))

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

	s <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				s <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending}
				cancel()
				if err := <-done; err != nil {
					slog.Warn("ipc server exited during stop", "error", err)
				}
				s <- svc.Status{State: svc.Stopped}
				return false, 0
			}
		case err := <-done:
			// 未收到停止指令却退出（如命名管道创建失败）。必须记日志并给非零
			// 退出码，否则表现为"启动成功后安静消失"，事件日志里没有任何线索。
			s <- svc.Status{State: svc.Stopped}
			if err != nil {
				slog.Error("ipc server exited unexpectedly", "error", err)
				return false, 1
			}
			return false, 0
		}
	}
}

func runIpc(ctx context.Context, homeDir string) error {
	inService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("check windows service: %w", err)
	}

	if inService {
		slog.Info("running as Windows service")
		return svc.Run(ipc.ServiceName, &windowsServiceHandler{homeDir: homeDir})
	}

	return runIpcForeground(ctx, homeDir)
}
