//go:build !windows

package ipc

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"syscall"
	"time"
)

func (s *Server) listenPlatform() error {
	os.Remove(s.ipcPath)

	var (
		listener net.Listener
		err      error
	)
	for range 30 {
		listener, err = net.ListenUnix("unix", &net.UnixAddr{
			Name: s.ipcPath,
			Net:  "unix",
		})
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EROFS) {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return fmt.Errorf("listen command sock: %w", err)
	}
	s.listener = listener

	if uid := os.Getuid(); uid != 0 {
		if err := os.Chmod(s.ipcPath, 0o600); err != nil {
			slog.Warn("chmod socket", "error", err)
		}
	}

	return nil
}

func (s *Server) cleanupPlatform() {
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.ipcPath)
}
