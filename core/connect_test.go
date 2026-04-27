package core

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"github.com/mapleafgo/singcast/translator"
)

const realConfigPath = "/home/mapleafgo/.local/share/cn.mapleafgo.clash_for_flutter/profiles/1777039692584.yaml"

// TestConnectivity_Google starts the service with a real config and verifies
// that google.com is reachable through the mixed proxy inbound,
// with connection events proving traffic actually went through singcast.
func TestConnectivity_Google(t *testing.T) {
	if _, err := os.Stat(realConfigPath); err != nil {
		t.Skipf("real config not found: %s", realConfigPath)
	}

	data, err := os.ReadFile(realConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	jsonContent, warns, err := translator.Translate(data)
	if err != nil {
		t.Fatalf("translate config: %v", err)
	}
	for _, w := range warns {
		t.Logf("WARN: %s", w)
	}

	mixedPort := extractMixedPort(t, jsonContent)
	if mixedPort == 0 {
		t.Fatal("no mixed inbound port found in config")
	}

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(jsonContent), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer Close()

	// Track connection events to prove traffic goes through singcast
	var (
		connMu      sync.Mutex
		connEvents  []string
		gotConnOpen bool
	)
	SetOnEvent(func(eventType int, jsonPayload string) {
		if eventType != EventConnections {
			return
		}
		connMu.Lock()
		defer connMu.Unlock()
		connEvents = append(connEvents, jsonPayload)
		// Connection open event has type "open"
		if !gotConnOpen {
			var msg struct {
				Reset bool `json:"reset"`
				Items []struct {
					Type int    `json:"type"`
					ID   string `json:"id"`
				} `json:"items"`
			}
			if json.Unmarshal([]byte(jsonPayload), &msg) == nil {
				for _, item := range msg.Items {
					if item.Type == 0 { // 0 = open
						gotConnOpen = true
						break
					}
				}
			}
		}
	})

	if err := Start(configPath); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer Stop()

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", mixedPort)
	if !waitForListen(t, proxyAddr, 10*time.Second) {
		t.Fatalf("proxy %s not listening after 10s", proxyAddr)
	}
	t.Logf("proxy listening on %s", proxyAddr)

	time.Sleep(2 * time.Second)

	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", proxyAddr))
	transport := &http.Transport{
		Proxy:                  http.ProxyURL(proxyURL), // fixed proxy, ignores env
		DialContext:            (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ResponseHeaderTimeout: 15 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://www.google.com", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("User-Agent", "singcast-connectivity-test/1.0")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET google.com through proxy: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	t.Logf("Status: %d", resp.StatusCode)
	t.Logf("Body (first 200 bytes): %.200s", string(body))

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify traffic actually went through singcast via connection events
	connMu.Lock()
	events := connEvents
	sawOpen := gotConnOpen
	connMu.Unlock()

	if !sawOpen {
		t.Error("no connection open event received — traffic may not have gone through singcast")
	} else {
		t.Logf("confirmed: received %d connection events, traffic went through singcast proxy", len(events))
	}
}

// TestConnectivity_SOCKS5 verifies that google.com is reachable through the
// mixed proxy inbound using the SOCKS5 protocol (instead of HTTP CONNECT).
func TestConnectivity_SOCKS5(t *testing.T) {
	if _, err := os.Stat(realConfigPath); err != nil {
		t.Skipf("real config not found: %s", realConfigPath)
	}

	data, err := os.ReadFile(realConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	jsonContent, warns, err := translator.Translate(data)
	if err != nil {
		t.Fatalf("translate config: %v", err)
	}
	for _, w := range warns {
		t.Logf("WARN: %s", w)
	}

	mixedPort := extractMixedPort(t, jsonContent)
	if mixedPort == 0 {
		t.Fatal("no mixed inbound port found in config")
	}

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(jsonContent), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer Close()

	var (
		connMu      sync.Mutex
		connEvents  []string
		gotConnOpen bool
	)
	SetOnEvent(func(eventType int, jsonPayload string) {
		if eventType != EventConnections {
			return
		}
		connMu.Lock()
		defer connMu.Unlock()
		connEvents = append(connEvents, jsonPayload)
		if !gotConnOpen {
			var msg struct {
				Reset bool `json:"reset"`
				Items []struct {
					Type int    `json:"type"`
					ID   string `json:"id"`
				} `json:"items"`
			}
			if json.Unmarshal([]byte(jsonPayload), &msg) == nil {
				for _, item := range msg.Items {
					if item.Type == 0 {
						gotConnOpen = true
						break
					}
				}
			}
		}
	})

	if err := Start(configPath); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer Stop()

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", mixedPort)
	if !waitForListen(t, proxyAddr, 10*time.Second) {
		t.Fatalf("proxy %s not listening after 10s", proxyAddr)
	}
	t.Logf("proxy listening on %s", proxyAddr)

	time.Sleep(2 * time.Second)

	// Dial through SOCKS5
	socksDialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, &net.Dialer{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("create SOCKS5 dialer: %v", err)
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return socksDialer.Dial(network, addr)
		},
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://www.google.com", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("User-Agent", "singcast-connectivity-test/1.0")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET google.com through SOCKS5 proxy: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	t.Logf("Status: %d", resp.StatusCode)
	t.Logf("Body (first 200 bytes): %.200s", string(body))

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	connMu.Lock()
	events := connEvents
	sawOpen := gotConnOpen
	connMu.Unlock()

	if !sawOpen {
		t.Error("no connection open event received — traffic may not have gone through singcast")
	} else {
		t.Logf("confirmed: received %d connection events, traffic went through SOCKS5 proxy", len(events))
	}
}

// TestConnectivity_HTTPS verifies that an HTTPS site is reachable through the
// mixed proxy inbound using HTTP CONNECT (the same as TestConnectivity_Google
// but targets a different HTTPS host to exercise TLS negotiation).
func TestConnectivity_HTTPS(t *testing.T) {
	if _, err := os.Stat(realConfigPath); err != nil {
		t.Skipf("real config not found: %s", realConfigPath)
	}

	data, err := os.ReadFile(realConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	jsonContent, warns, err := translator.Translate(data)
	if err != nil {
		t.Fatalf("translate config: %v", err)
	}
	for _, w := range warns {
		t.Logf("WARN: %s", w)
	}

	mixedPort := extractMixedPort(t, jsonContent)
	if mixedPort == 0 {
		t.Fatal("no mixed inbound port found in config")
	}

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(jsonContent), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer Close()

	var (
		connMu      sync.Mutex
		connEvents  []string
		gotConnOpen bool
	)
	SetOnEvent(func(eventType int, jsonPayload string) {
		if eventType != EventConnections {
			return
		}
		connMu.Lock()
		defer connMu.Unlock()
		connEvents = append(connEvents, jsonPayload)
		if !gotConnOpen {
			var msg struct {
				Reset bool `json:"reset"`
				Items []struct {
					Type int    `json:"type"`
					ID   string `json:"id"`
				} `json:"items"`
			}
			if json.Unmarshal([]byte(jsonPayload), &msg) == nil {
				for _, item := range msg.Items {
					if item.Type == 0 {
						gotConnOpen = true
						break
					}
				}
			}
		}
	})

	if err := Start(configPath); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer Stop()

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", mixedPort)
	if !waitForListen(t, proxyAddr, 10*time.Second) {
		t.Fatalf("proxy %s not listening after 10s", proxyAddr)
	}
	t.Logf("proxy listening on %s", proxyAddr)

	time.Sleep(2 * time.Second)

	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", proxyAddr))
	transport := &http.Transport{
		Proxy:                  http.ProxyURL(proxyURL),
		DialContext:            (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ResponseHeaderTimeout: 15 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://www.gstatic.com/generate_204", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("User-Agent", "singcast-connectivity-test/1.0")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET gstatic.com through proxy: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("Status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
	}

	connMu.Lock()
	events := connEvents
	sawOpen := gotConnOpen
	connMu.Unlock()

	if !sawOpen {
		t.Error("no connection open event received — traffic may not have gone through singcast")
	} else {
		t.Logf("confirmed: received %d connection events, HTTPS traffic went through proxy", len(events))
	}
}

// extractMixedPort reads the mixed inbound listen_port from sing-box JSON config.
func extractMixedPort(t *testing.T, jsonStr string) uint16 {
	t.Helper()
	var cfg struct {
		Inbounds []struct {
			Type       string `json:"type"`
			ListenPort uint16 `json:"listen_port"`
		} `json:"inbounds"`
	}
	if err := parseJSONTo(jsonStr, &cfg); err != nil {
		t.Fatalf("parse config for port: %v", err)
	}
	for _, in := range cfg.Inbounds {
		if in.Type == "mixed" {
			return in.ListenPort
		}
	}
	return 0
}

func parseJSONTo(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}

// waitForListen polls until the address is accepting TCP connections.
func waitForListen(t *testing.T, addr string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
