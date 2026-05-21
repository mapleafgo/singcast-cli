//go:build windows

package ipc

import (
	"fmt"

	"github.com/tailscale/go-winio"
)

const pipeSDDL = "D:(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;IU)"

func (s *Server) listenPlatform() error {
	l, err := winio.ListenPipe(s.ipcPath, &winio.PipeConfig{
		SecurityDescriptor: pipeSDDL,
	})
	if err != nil {
		return fmt.Errorf("listen named pipe: %w", err)
	}
	s.listener = l
	return nil
}

func (s *Server) cleanupPlatform() {
	if s.listener != nil {
		s.listener.Close()
	}
}
