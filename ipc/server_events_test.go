//go:build !windows

package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mapleafgo/singcast/core"
)

func freeTCPPortForIPC(t *testing.T) uint16 {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen ephemeral port: %v", err)
	}
	defer listener.Close()
	return uint16(listener.Addr().(*net.TCPAddr).Port)
}

// 未配置 clash_api 时，IPC（桌面 GUI 走的就是这条通道）仍必须推送实时事件通知。
// GUI 的流量/连接数/内存/启动时间来自 event.trafficUpdate，节点延迟来自
// event.urlTest；二者都不应要求启用对外 API。
func TestServer_PushesTrafficUpdateWithoutClashAPI(t *testing.T) {
	if _, err := exec.LookPath("setfacl"); err != nil {
		t.Skip("setfacl 不可用，跳过需要 socket 授权链路的测试")
	}

	sockPath := filepath.Join(t.TempDir(), "command.sock")
	config := fmt.Sprintf(`{
		"log": {"level": "error"},
		"inbounds": [{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": %d}],
		"outbounds": [{"type": "direct", "tag": "DIRECT"}],
		"route": {"final": "DIRECT"}
	}`, freeTCPPortForIPC(t))

	svc := core.NewService()
	if err := svc.Init(fmt.Sprintf(`{"home_dir":%q}`, t.TempDir())); err != nil {
		t.Fatalf("init core: %v", err)
	}
	defer svc.Destroy()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewServer(svc, sockPath)
	go func() { _ = server.Run(ctx) }()

	conn := dialUntilReady(t, sockPath)
	defer conn.Close()

	if err := svc.StartWithContent(config, ""); err != nil {
		t.Fatalf("start core: %v", err)
	}
	defer svc.Stop()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var msg struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Method == NotifyTrafficUpdate {
			return
		}
	}
	t.Fatal("5s 内没有通过 IPC 收到 event.trafficUpdate 通知")
}

// dialUntilReady 在 socket 就绪前重试连接，避免与 Run 的监听竞态。
func dialUntilReady(t *testing.T, sockPath string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	dialer := &net.Dialer{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		conn, err := dialer.DialContext(context.Background(), "unix", sockPath)
		if err == nil {
			return conn
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("5s 内无法连接 IPC socket %s", sockPath)
	return nil
}
