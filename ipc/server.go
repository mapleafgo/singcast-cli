package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/mapleafgo/singcast/core"
)

// Server is the IPC server that listens for GUI connections and handles requests.
type Server struct {
	handler  *Handler
	ipcPath  string
	listener net.Listener

	mu     sync.Mutex
	conn   net.Conn
	encMu  sync.Mutex
	writer *bufio.Writer
}

// NewServer creates a new IPC server listening at ipcPath.
// On desktop pass ipc.IpcPath(); on mobile pass an App Group shared path
// so a second process (the app) can drive the same kernel over JSON-RPC.
func NewServer(svc *core.Service, ipcPath string) *Server {
	return &Server{
		ipcPath: ipcPath,
		handler: NewHandler(svc),
	}
}

// Run starts listening and blocks until the context is cancelled or a fatal error occurs.
func (s *Server) Run(ctx context.Context) error {
	// Setup IPC event bridge: core events → JSON-RPC notifications
	s.handler.svc.SetOnEvent(func(eventType int32, payload string) {
		s.onCoreEvent(eventType, payload)
	})

	if err := s.listenPlatform(); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer s.cleanupPlatform()

	slog.Info("IPC server listening", "path", s.ipcPath)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-ctx.Done()
		if s.listener != nil {
			s.listener.Close()
		}
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				slog.Warn("accept error", "error", err)
				continue
			}
		}
		slog.Info("GUI connected", "remote", conn.RemoteAddr())
		s.serveConnection(ctx, conn)
		slog.Info("GUI disconnected")

		// TUN protection: if kernel is still running after GUI disconnects,
		// keep the service alive — the TUN device must not be destroyed.
		if s.handler.svc.State() == core.StateRunning {
			slog.Info("kernel still running, keeping service alive for TUN protection")
			continue
		}

		slog.Info("kernel not running, shutting down service")
		return nil
	}
}

func (s *Server) serveConnection(ctx context.Context, conn net.Conn) {
	s.mu.Lock()
	s.conn = conn
	s.writer = bufio.NewWriter(conn)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.conn = nil
		s.writer = nil
		s.mu.Unlock()
		conn.Close()
	}()

	dec := json.NewDecoder(conn)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var req JSONRPCRequest
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return
			}
			slog.Warn("decode error", "error", err)
			return
		}
		s.handleRequest(&req) //nolint:contextcheck // IPC 请求没有外部 context 来源
	}
}

func (s *Server) handleRequest(req *JSONRPCRequest) {
	if req.IsNotification() {
		s.handler.Handle(req)
	} else {
		go func() {
			resp := s.handler.Handle(req)
			s.sendResponse(resp)
		}()
	}
}

func (s *Server) sendResponse(resp JSONRPCResponse) {
	s.encMu.Lock()
	defer s.encMu.Unlock()

	if s.writer == nil {
		return
	}

	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("marshal response", "error", err)
		return
	}
	data = append(data, '\n')

	if _, err := s.writer.Write(data); err != nil {
		slog.Warn("write response", "error", err)
		return
	}
	_ = s.writer.Flush()
}

func (s *Server) sendNotification(method string, params any) {
	s.encMu.Lock()
	defer s.encMu.Unlock()

	if s.writer == nil {
		return
	}

	notif := Notification{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(notif)
	if err != nil {
		slog.Error("marshal notification", "error", err)
		return
	}
	data = append(data, '\n')

	if _, err := s.writer.Write(data); err != nil {
		slog.Warn("write notification", "error", err)
		return
	}
	_ = s.writer.Flush()
}

// onCoreEvent bridges core.Service events to JSON-RPC notifications.
func (s *Server) onCoreEvent(eventType int32, payload string) {
	switch eventType {
	case core.EventLog:
		s.sendNotification(NotifyLog, json.RawMessage(payload))
	case core.EventURLTest:
		s.sendNotification(NotifyURLTest, nil)
	case core.EventModeUpdate:
		s.sendNotification(NotifyModeUpdate, map[string]string{"mode": payload})
	case core.EventConnEvent:
		s.sendNotification(NotifyConnEvent, json.RawMessage(payload))
	case core.EventStateChange:
		s.sendNotification(NotifyStateUpdate, StateUpdatePayload{State: payload})
	case core.EventStats:
		s.sendNotification(NotifyTrafficUpdate, json.RawMessage(payload))
	}
}

// IpcPath returns the IPC socket/pipe path based on the current working directory.
func IpcPath() string {
	// 服务模式（systemd）通过环境变量指定固定 socket 路径
	if p := os.Getenv("SINGCAST_IPC_PATH"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return `\\.\pipe\singcast`
	}
	homeDir, err := os.Getwd()
	if err != nil {
		homeDir = "."
	}
	return filepath.Join(homeDir, "command.sock")
}
