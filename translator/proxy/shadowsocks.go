package proxy

import (
	"fmt"
	"sort"
	"strings"
)

// supportedSSCiphers lists the Shadowsocks ciphers supported by sing-box.
var supportedSSCiphers = map[string]bool{
	"aes-128-gcm":                      true,
	"aes-192-gcm":                      true,
	"aes-256-gcm":                      true,
	"chacha20-ietf-poly1305":           true,
	"xchacha20-ietf-poly1305":          true,
	"2022-blake3-aes-128-gcm":          true,
	"2022-blake3-aes-256-gcm":          true,
	"2022-blake3-chacha20-poly1305":    true,
}

// unsupportedSSCiphers lists ciphers that sing-box does not support.
var unsupportedSSCiphers = map[string]bool{
	"rc4-md5":            true,
	"chacha20-ietf":      true,
	"aes-128-cfb":        true,
	"aes-192-cfb":        true,
	"aes-256-cfb":        true,
	"aes-128-ctr":        true,
	"aes-192-ctr":        true,
	"aes-256-ctr":        true,
	"none":               true,
	"chacha20":           true,
	"aes-128-ofb":        true,
	"aes-192-ofb":        true,
	"aes-256-ofb":        true,
}

// TranslateShadowsocks translates a mihomo Shadowsocks proxy config to a sing-box outbound.
// Returns nil if the cipher is unsupported.
// See mapping doc section B.8.
func TranslateShadowsocks(m map[string]any, warn func(string)) map[string]any {
	cipher := GetStr(m, "cipher")
	if cipher == "" {
		warn("shadowsocks: missing cipher")
		return nil
	}

	// Check for unsupported ciphers
	if unsupportedSSCiphers[cipher] {
		warn("shadowsocks: cipher '" + cipher + "' is not supported by sing-box, skipping")
		return nil
	}

	// Warn about unknown ciphers that are not in either list
	if !supportedSSCiphers[cipher] {
		warn("shadowsocks: cipher '" + cipher + "' is not recognized, may not work")
	}

	outbound := map[string]any{
		"type": "shadowsocks",
		"tag":  GetStr(m, "name"),
	}
	ApplyCommonFields(m, outbound)

	// Cipher -> method
	outbound["method"] = cipher

	// Password
	if password := GetStr(m, "password"); password != "" {
		outbound["password"] = password
	}

	// Plugin
	if plugin := GetStr(m, "plugin"); plugin != "" {
		outbound["plugin"] = plugin
	}

	// Plugin opts: convert from object to SIP003 string format
	// e.g., {mode: "tls", host: "bing.com"} -> "obfs=tls;host=bing.com"
	pluginOpts := GetMap(m, "plugin-opts")
	if pluginOpts != nil {
		outbound["plugin_opts"] = buildSIP003Opts(pluginOpts)
	}

	// UDP over TCP
	if uot := GetBool(m, "udp-over-tcp"); uot {
		outbound["udp_over_tcp"] = true
	}

	// Multiplex
	applyMultiplex(m, outbound, warn)

	return outbound
}

// sip003KeyMap maps mihomo plugin-opts keys to SIP003 standard keys.
var sip003KeyMap = map[string]string{
	"mode": "obfs",
	"host": "obfs-host",
}

// buildSIP003Opts converts a plugin-opts object to a SIP003 format string.
// e.g., {mode: "tls", host: "bing.com"} -> "obfs=tls;obfs-host=bing.com"
func buildSIP003Opts(opts map[string]any) string {
	if len(opts) == 0 {
		return ""
	}

	parts := make([]string, 0, len(opts))
	keys := make([]string, 0, len(opts))
	for k := range opts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := opts[k]
		if mapped, ok := sip003KeyMap[k]; ok {
			k = mapped
		}
		parts = append(parts, k+"="+fmt.Sprintf("%v", v))
	}
	return strings.Join(parts, ";")
}
