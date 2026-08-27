package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

func TestCloseConnectionsClosesActiveProxyConnections(t *testing.T) {
	echoListener := listenLocal(t)
	defer echoListener.Close()
	echoAddr := echoListener.Addr().String()
	go func() {
		for {
			conn, err := echoListener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	mixedPort := freeTCPPort(t)
	config := fmt.Sprintf(`{
		"log": {"level": "error"},
		"inbounds": [{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": %d}],
		"outbounds": [{"type": "direct", "tag": "DIRECT"}],
		"route": {"final": "DIRECT"}
	}`, mixedPort)

	svc := NewService()
	if err := svc.Init(initJSON(filepath.Join(t.TempDir(), "home"))); err != nil {
		t.Fatalf("init service: %v", err)
	}
	defer svc.Destroy()
	if err := svc.StartWithContent(config, ""); err != nil {
		t.Fatalf("start service: %v", err)
	}
	defer svc.Stop()

	mixedAddr := fmt.Sprintf("127.0.0.1:%d", mixedPort)
	if !waitForListen(t, mixedAddr, 5*time.Second) {
		t.Fatalf("mixed proxy %s did not listen", mixedAddr)
	}

	dialer, err := proxy.SOCKS5("tcp", mixedAddr, nil, proxy.Direct)
	if err != nil {
		t.Fatalf("create SOCKS5 dialer: %v", err)
	}
	client, err := dialer.Dial("tcp", echoAddr)
	if err != nil {
		t.Fatalf("dial through proxy: %v", err)
	}
	defer client.Close()
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write through proxy: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set echo deadline: %v", err)
	}
	echo := make([]byte, 4)
	if _, err := io.ReadFull(client, echo); err != nil {
		t.Fatalf("read echo: %v", err)
	}

	if err := svc.CloseConnections(); err != nil {
		t.Fatalf("close connections: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set close deadline: %v", err)
	}
	if _, err := client.Read(echo); !errors.Is(err, io.EOF) {
		t.Fatalf("connection remains open after CloseConnections, read error = %v", err)
	}
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on local ephemeral port: %v", err)
	}
	return listener
}

func freeTCPPort(t *testing.T) uint16 {
	t.Helper()
	listener := listenLocal(t)
	defer listener.Close()
	return uint16(listener.Addr().(*net.TCPAddr).Port)
}
