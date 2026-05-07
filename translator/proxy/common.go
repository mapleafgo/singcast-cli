package proxy

import (
	"fmt"
	"strconv"
	"strings"
)

// GetStr retrieves a string value from a map. Returns "" if key is missing or not a string.
func GetStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// GetInt retrieves an int value from a map. Returns 0 if key is missing or not convertible.
func GetInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

// GetBool retrieves a bool value from a map. Returns false if key is missing or not a bool.
func GetBool(m map[string]any, key string) bool {
	b, _ := m[key].(bool)
	return b
}

// GetMap retrieves a map[string]any value from a map. Returns nil if key is missing or not a map.
func GetMap(m map[string]any, key string) map[string]any {
	v, ok := m[key]
	if !ok {
		return nil
	}
	sub, ok := v.(map[string]any)
	if ok {
		return sub
	}
	return nil
}

// GetStrSlice retrieves a []string value from a map. Returns nil if key is missing.
func GetStrSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return nil
	}
}

// ApplyCommonFields applies standard mihomo-to-sing-box field mappings:
// name -> tag, server -> server, port -> server_port, and dial fields.
func ApplyCommonFields(src map[string]any, dst map[string]any) {
	dst["tag"] = GetStr(src, "name")
	dst["server"] = GetStr(src, "server")
	if port := GetInt(src, "port"); port != 0 {
		dst["server_port"] = port
	}

	// Dial fields
	if iface := GetStr(src, "interface-name"); iface != "" {
		dst["bind_interface"] = iface
	}
	if mark := GetInt(src, "routing-mark"); mark > 0 {
		dst["routing_mark"] = mark
	}
	if tfo := GetBool(src, "tfo"); tfo {
		dst["tcp_fast_open"] = true
	}
	if mptcp := GetBool(src, "mptcp"); mptcp {
		dst["tcp_multi_path"] = true
	}
	if detour := GetStr(src, "dialer-proxy"); detour != "" {
		dst["detour"] = detour
	}
}

// SecondsToDuration converts an integer number of seconds to a Go duration string.
// e.g., 300 -> "5m", 30 -> "30s", 3600 -> "1h0m0s"
func SecondsToDuration(seconds int) string {
	if seconds <= 0 {
		return "0s"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60

	var parts []string
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if s > 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}
	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, "")
}

// ParseBandwidth parses a bandwidth string like "50 Mbps" or "200 Mbps" and returns the integer Mbps value.
// If the string is just a number, it is returned directly.
// Returns 0 if parsing fails.
func ParseBandwidth(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Try plain integer first
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}

	// Try to parse "<number> <unit>" format
	s = strings.ToLower(strings.TrimSpace(s))

	// Remove common unit suffixes (longest first to avoid partial matches)
	for _, suffix := range []string{"gbps", "mbps", "kbps", "gb", "mb", "kb"} {
		s = strings.TrimSuffix(s, suffix)
	}
	s = strings.TrimSpace(s)

	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}

// applyMultiplex translates mihomo smux configuration to sing-box multiplex.
// See mapping doc section B.4.
func applyMultiplex(m map[string]any, outbound map[string]any) {
	smux := GetMap(m, "smux")
	if smux == nil {
		return
	}

	if !GetBool(smux, "enabled") {
		return
	}

	multiplex := map[string]any{
		"enabled": true,
	}

	if protocol := GetStr(smux, "protocol"); protocol != "" {
		multiplex["protocol"] = protocol
	}
	if maxConn := GetInt(smux, "max-connections"); maxConn > 0 {
		multiplex["max_connections"] = maxConn
	}
	if maxStreams := GetInt(smux, "max-streams"); maxStreams > 0 {
		multiplex["max_streams"] = maxStreams
	}
	if padding := GetBool(smux, "padding"); padding {
		multiplex["padding"] = true
	}

	// Brutal options
	brutalOpts := GetMap(smux, "brutal-opts")
	if brutalOpts != nil {
		brutal := map[string]any{}
		if GetBool(brutalOpts, "enabled") {
			brutal["enabled"] = true
		}
		if up := GetStr(brutalOpts, "up"); up != "" {
			if mbps := ParseBandwidth(up); mbps > 0 {
				brutal["up_mbps"] = mbps
			}
		}
		if down := GetStr(brutalOpts, "down"); down != "" {
			if mbps := ParseBandwidth(down); mbps > 0 {
				brutal["down_mbps"] = mbps
			}
		}
		if len(brutal) > 0 {
			multiplex["brutal"] = brutal
		}
	}

	outbound["multiplex"] = multiplex
}

// BuildAlwaysOnTLS creates a TLS config for protocols that always require TLS
// (e.g., Hysteria2, TUIC).
func BuildAlwaysOnTLS(m map[string]any) map[string]any {
	tls := map[string]any{"enabled": true}
	if sni := GetStr(m, "sni"); sni != "" {
		tls["server_name"] = sni
	}
	if GetBool(m, "skip-cert-verify") {
		tls["insecure"] = true
	}
	if alpn := GetStrSlice(m, "alpn"); alpn != nil {
		tls["alpn"] = alpn
	}
	return tls
}
