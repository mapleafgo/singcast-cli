package translator

import (
	"maps"
	"net/netip"
	"strings"

	"github.com/mapleafgo/singcast/translator/proxy"
)

func translateProxies(cfg *RawConfig, t *translation) []map[string]any {
	var outbounds []map[string]any

	for _, p := range cfg.Proxy {
		proxyType, _ := p["type"].(string)
		name, _ := p["name"].(string)
		if name == "" {
			continue
		}
		if isInvalidHealthCheckEndpoint(p) {
			markInvalidHealthCheckProxy(t, name)
		}
		if !hasProxyEndpoint(p) {
			t.warn("proxy \"" + name + "\": missing server or port, degraded to stub")
			outbounds = append(outbounds, makeStubOutbound(name, proxyType))
			t.stubTags[name] = proxyType
			t.proxyTags[name] = true
			markInvalidHealthCheckProxy(t, name)
			continue
		}
		outbound := translateOneProxy(p, t.warn, cfg.GlobalFingerprint)
		if outbound == nil {
			// Create a socks stub so unsupported nodes stay visible in the UI.
			// The stub points to 127.0.0.1:1 — a dead endpoint that fails on
			// any connection attempt, making it clear the node is non-functional.
			outbound = makeStubOutbound(name, proxyType)
			t.stubTags[name] = proxyType
			markInvalidHealthCheckProxy(t, name)
		}
		tag, _ := outbound["tag"].(string)
		if tag == "" {
			continue
		}
		t.proxyTags[tag] = true
		outbounds = append(outbounds, outbound)
	}

	return outbounds
}

func isInvalidHealthCheckEndpoint(p map[string]any) bool {
	server := strings.TrimSpace(proxy.GetStr(p, "server"))
	if server == "" {
		return false
	}
	if strings.HasPrefix(server, "[") && strings.HasSuffix(server, "]") {
		server = strings.TrimPrefix(strings.TrimSuffix(server, "]"), "[")
	}
	if strings.EqualFold(server, "localhost") {
		return true
	}
	if addr, err := netip.ParseAddr(server); err == nil {
		if addr.IsLoopback() || addr.IsUnspecified() {
			return true
		}
	}

	port := proxy.GetInt(p, "port")
	if port != 0 && (port < 1 || port > 65535) {
		return true
	}
	return false
}

func markInvalidHealthCheckProxy(t *translation, name string) {
	if t.invalidHealthCheckProxyTags == nil {
		t.invalidHealthCheckProxyTags = make(map[string]bool)
	}
	t.invalidHealthCheckProxyTags[name] = true
}

func hasProxyEndpoint(p map[string]any) bool {
	if proxy.GetStr(p, "server") == "" {
		return false
	}
	return proxy.GetInt(p, "port") != 0 || proxy.GetStr(p, "ports") != ""
}

// makeStubOutbound creates a non-functional socks outbound for unsupported proxies.
// The node appears in the Clash API UI but fails on use or health check.
// username 用于在转换产物中持久化原始协议标记，保存配置后再启动时仍能恢复
// stubTags；stub 指向死端点，不会真正发起 Socks 认证，因此该字段只是内部标记。
func makeStubOutbound(name, proxyType string) map[string]any {
	return map[string]any{
		"type":        "socks",
		"tag":         name,
		"version":     "5",
		"server":      "127.0.0.1",
		"server_port": 1,
		"username":    "unsupported:" + proxyType,
	}
}

func translateOneProxy(m map[string]any, warn func(string), globalFingerprint string) map[string]any {
	// Apply global-client-fingerprint as fallback when proxy has no per-proxy fingerprint
	if globalFingerprint != "" {
		if _, ok := m["client-fingerprint"]; !ok {
			m = maps.Clone(m)
			m["client-fingerprint"] = globalFingerprint
		}
	}

	proxyType, _ := m["type"].(string)

	switch proxyType {
	case "vless":
		return proxy.TranslateVLESS(m, warn)
	case "vmess":
		return proxy.TranslateVMess(m, warn)
	case "trojan":
		return proxy.TranslateTrojan(m, warn)
	case "ss":
		return proxy.TranslateShadowsocks(m, warn)
	case "shadowsocks":
		return proxy.TranslateShadowsocks(m, warn)
	case "hysteria2":
		return proxy.TranslateHysteria2(m, warn)
	case "tuic":
		return proxy.TranslateTUIC(m, warn)
	case "anytls":
		return proxy.TranslateAnyTLS(m, warn)
	case "wireguard":
		// WireGuard outbound removed in sing-box 1.13.0, replaced by endpoint config
		return proxy.TranslateUnsupported("wireguard", m, warn)
	case "socks5":
		return proxy.TranslateSOCKS(m, warn)
	case "http":
		return proxy.TranslateHTTP(m, warn)
	case "ssr":
		return proxy.TranslateUnsupported("ssr", m, warn)
	case "snell":
		return proxy.TranslateUnsupported("snell", m, warn)
	case "ssh":
		return proxy.TranslateUnsupported("ssh", m, warn)
	case "hysteria":
		return proxy.TranslateHysteria(m, warn)
	case "mieru":
		return proxy.TranslateUnsupported("mieru", m, warn)
	case "sudoku":
		return proxy.TranslateUnsupported("sudoku", m, warn)
	case "masque":
		return proxy.TranslateUnsupported("masque", m, warn)
	case "trusttunnel":
		return proxy.TranslateUnsupported("trusttunnel", m, warn)
	default:
		warn("unknown proxy type: " + proxyType)
		return nil
	}
}
