package translator

import (
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
		if !hasProxyEndpoint(p) {
			t.warn("proxy \"" + name + "\": missing server or port, degraded to stub")
			outbounds = append(outbounds, makeStubOutbound(name))
			t.stubTags[name] = proxyType
			t.proxyTags[name] = true
			continue
		}
		outbound := translateOneProxy(p, t.warn, cfg.GlobalFingerprint)
		if outbound == nil {
			// Create a socks stub so unsupported nodes stay visible in the UI.
			// The stub points to 127.0.0.1:1 — a dead endpoint that fails on
			// any connection attempt, making it clear the node is non-functional.
			outbound = makeStubOutbound(name)
			t.stubTags[name] = proxyType
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

func hasProxyEndpoint(p map[string]any) bool {
	if proxy.GetStr(p, "server") == "" {
		return false
	}
	return proxy.GetInt(p, "port") != 0 || proxy.GetStr(p, "ports") != ""
}

// makeStubOutbound creates a non-functional socks outbound for unsupported proxies.
// The node appears in the Clash API UI but fails on use or health check.
func makeStubOutbound(name string) map[string]any {
	return map[string]any{
		"type":        "socks",
		"tag":         name,
		"version":     "5",
		"server":      "127.0.0.1",
		"server_port": 1,
	}
}

func cloneMap(m map[string]any) map[string]any {
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func translateOneProxy(m map[string]any, warn func(string), globalFingerprint string) map[string]any {
	// Apply global-client-fingerprint as fallback when proxy has no per-proxy fingerprint
	if globalFingerprint != "" {
		if _, ok := m["client-fingerprint"]; !ok {
			m = cloneMap(m)
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
