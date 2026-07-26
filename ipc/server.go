package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mapleafgo/singcast/core"
)

// Server is the IPC server that listens for GUI connections and handles requests.
type Server struct {
	handler  *Handler
	ipcPath  string
	listener net.Listener

	// active 指向当前 GUI 连接，供通知广播与 ctx 取消时中断阻塞的 Decode。
	active atomic.Pointer[clientConn]
}

// clientConn 是一次 GUI 连接的写端。每次连接新建一个实例，请求的响应写回
// 发起它的那个实例——共用一个 writer 字段会让旧连接的慢请求（如
// startWithContent）在 GUI 重连后把响应写进新连接，撕裂 JSON 流。
type clientConn struct {
	conn   net.Conn
	mu     sync.Mutex
	writer *bufio.Writer
}

// send 序列化 v 并写入本连接，一行一条消息。多 goroutine 可并发调用。
// kind 用于日志定位是哪类消息写失败（响应还是某种通知），不影响传输内容。
func (c *clientConn) send(kind string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Error("marshal ipc message", "kind", kind, "error", err)
		return
	}
	data = append(data, '\n')

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.writer.Write(data); err != nil {
		slog.Warn("write ipc message", "kind", kind, "bytes", len(data), "error", err)
		return
	}
	_ = c.writer.Flush()
}

// NewServer creates a new IPC server listening at ipcPath.
// On desktop pass ipc.IpcPath(homeDir); on mobile pass an App Group shared path
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

	if err := s.listenPlatform(ctx); err != nil {
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
		// 同时关掉在用连接：serveConnection 阻塞在 Decode 上，只关 listener
		// 无法让它返回，systemd stop 会一直等到 TimeoutStopSec 后 SIGKILL。
		if cc := s.active.Load(); cc != nil {
			cc.conn.Close()
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
	cc := &clientConn{conn: conn, writer: bufio.NewWriter(conn)}
	s.active.Store(cc)

	defer func() {
		s.active.CompareAndSwap(cc, nil)
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
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			slog.Warn("decode error", "error", err)
			return
		}
		s.handleRequest(cc, &req) //nolint:contextcheck // IPC 请求没有外部 context 来源
	}
}

// handleRequest 分发一条请求。带 id 的请求异步处理，避免慢请求
// （startWithContent 可耗时数秒）阻塞同连接上后续请求的读取；
// 通知无需响应，同步处理以保持相对顺序。
//
// 入口/出口各打一条 Debug 并带耗时：startWithContent、testGroupDelay 这类
// 操作是秒级的，只有入口日志时无法区分"仍在处理"和"已卡死"。
func (s *Server) handleRequest(cc *clientConn, req *JSONRPCRequest) {
	if req.IsNotification() {
		slog.Debug("ipc notification", "method", req.Method)
		s.handler.Handle(req)
		return
	}

	slog.Debug("ipc request", "method", req.Method, "id", req.ID)
	go func() {
		start := time.Now()
		resp := s.handler.Handle(req)
		cc.send("response", resp)
		slog.Debug("ipc request done", "method", req.Method, "id", req.ID,
			"elapsed_ms", time.Since(start).Milliseconds(), "error", resp.Error != nil)
	}()
}

func (s *Server) sendNotification(method string, params any) {
	cc := s.active.Load()
	if cc == nil {
		return
	}
	cc.send(method, Notification{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  params,
	})
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

// IpcPath 返回 homeDir 下的 IPC socket 路径（Windows 为固定命名管道）。
// 优先级：SINGCAST_IPC_PATH 环境变量（systemd 服务模式用它指定 /run 下的固定路径）
// > homeDir/command.sock。homeDir 为空时回退到当前工作目录。
func IpcPath(homeDir string) string {
	if p := os.Getenv("SINGCAST_IPC_PATH"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return `\\.\pipe\singcast`
	}
	if homeDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		homeDir = cwd
	}
	return filepath.Join(homeDir, "command.sock")
}
