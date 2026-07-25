//go:build !windows

package ipc

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
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

	// Set restrictive permissions: only the owner can connect.
	if err := os.Chmod(s.ipcPath, 0o600); err != nil {
		slog.Warn("chmod socket", "error", err)
	}

	dirInfo, err := os.Stat(filepath.Dir(s.ipcPath))
	if err != nil {
		slog.Warn("stat socket dir for chown", "error", err)
	} else if stat, ok := dirInfo.Sys().(*syscall.Stat_t); ok && stat.Uid != 0 {
		if err := os.Chown(s.ipcPath, int(stat.Uid), int(stat.Gid)); err != nil {
			slog.Warn("chown socket", "error", err)
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
