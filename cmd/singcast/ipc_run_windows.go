//go:build windows

package main

import (
	"context"
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

	svcInst := core.NewService()
	if err := svcInst.Init(fmt.Sprintf(`{"home_dir":%q}`, w.homeDir)); err != nil {
		slog.Error("init service", "error", err)
		s <- svc.Status{State: svc.Stopped}
		return true, 1
	}

	srv := ipc.NewServer(svcInst)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
				<-done
				s <- svc.Status{State: svc.Stopped}
				return false, 0
			}
		case <-done:
			s <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
}

func runIpc(homeDir string) error {
	inService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("check windows service: %w", err)
	}

	if inService {
		slog.Info("running as Windows service")
		return svc.Run(ipc.ServiceName, &windowsServiceHandler{homeDir: homeDir})
	}

	return runIpcForeground(homeDir)
}
