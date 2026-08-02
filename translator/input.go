package translator

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// proxyListYAML 只序列化 proxies 字段，避免把 RawConfig 的零值字段全部输出。
type proxyListYAML struct {
	Proxies []map[string]any `yaml:"proxies"`
}

// NormalizeInput 统一订阅输入：base64 解码；URI 列表组装为 mihomo YAML。
func NormalizeInput(data []byte) ([]byte, error) {
	if decoded, ok := decodeBase64Input(data); ok {
		data = decoded
	}
	if isProxyURIList(data) {
		return buildClashYAMLFromURIs(data)
	}
	return data, nil
}

func decodeBase64Input(data []byte) ([]byte, bool) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, false
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		decoded, err := enc.DecodeString(trimmed)
		if err != nil {
			continue
		}
		if looksLikeSubscription(decoded) {
			return decoded, true
		}
	}
	return nil, false
}

func looksLikeSubscription(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "://") {
		return true
	}
	if strings.HasPrefix(trimmed, "{") && json.Valid([]byte(trimmed)) {
		return true
	}
	return strings.Contains(trimmed, "proxies:")
}

func isProxyURIList(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return false
	}
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.Contains(line, "://") {
			return false
		}
	}
	return true
}

func buildClashYAMLFromURIs(data []byte) ([]byte, error) {
	cfg, err := buildRawConfigFromURIs(data)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(proxyListYAML{Proxies: cfg.Proxy})
}

// buildRawConfigFromURIs 逐行解析代理 URI，直接构造 RawConfig。
// 跳过 YAML 序列化往返，供 Convert 直接调用。
// 默认值参考 mihomo DefaultRawConfig()，确保 URI 列表转换结果与
// mihomo 导入行为一致（关键：IPv6=true 影响 DNS strategy 与 TUN inet6 地址）。
func buildRawConfigFromURIs(data []byte) (*RawConfig, error) {
	proxies := []map[string]any{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if p := parseProxyURI(line); p != nil {
			proxies = append(proxies, p)
		}
	}
	if len(proxies) == 0 {
		return nil, fmt.Errorf("no valid proxies found in subscription")
	}
	return &RawConfig{
		BindAddress:     "*",
		Mode:            "rule",
		LogLevel:        "info",
		IPv6:            true,
		FindProcessMode: "strict",
		DNS: RawDNS{
			Enable:       true,
			EnhancedMode: "redir-host",
			FakeIPRange:  "198.18.0.1/16",
			DefaultNameserver: []string{
				"114.114.114.114",
				"223.5.5.5",
				"8.8.8.8",
				"1.0.0.1",
			},
			NameServer: []string{
				"https://doh.pub/dns-query",
				"tls://223.5.5.5:853",
			},
			FallbackFilter: RawFallbackFilter{
				GeoIP:     true,
				GeoIPCode: "CN",
			},
			FakeIPFilter: []string{
				"dns.msftnsci.com",
				"www.msftnsci.com",
				"www.msftconnecttest.com",
			},
			FakeIPFilterMode: "blacklist",
		},
		Proxy: proxies,
	}, nil
}

func parseProxyURI(uri string) map[string]any {
	if !strings.Contains(uri, "://") {
		return nil
	}
	typeEnd := strings.Index(uri, "://")
	scheme := strings.ToLower(uri[:typeEnd])
	rest := uri[typeEnd+3:]
	name := "proxy"
	body := rest
	if hashIdx := strings.Index(rest, "#"); hashIdx >= 0 {
		if decoded, err := url.QueryUnescape(rest[hashIdx+1:]); err == nil && decoded != "" {
			name = decoded
		}
		body = rest[:hashIdx]
	}
	switch scheme {
	case "ss":
		return parseSS(body, name)
	case "vmess":
		return parseVmess(body, name)
	case "vless":
		return parseVless(body, name)
	case "trojan":
		return parseTrojan(body, name)
	case "hysteria2", "hy2":
		return parseHysteria2(body, name)
	default:
		return nil
	}
}

func parseSS(body, name string) map[string]any {
	var decoded string
	var serverPort []string
	if strings.Contains(body, "@") {
		atIdx := strings.Index(body, "@")
		dec, err := base64Decode(body[:atIdx])
		if err != nil {
			return nil
		}
		decoded = dec
		serverPort = strings.Split(strings.Split(body[atIdx+1:], "?")[0], ":")
	} else {
		dec, err := base64Decode(strings.Split(body, "?")[0])
		if err != nil {
			return nil
		}
		decoded = dec
		atIdx := strings.LastIndex(decoded, "@")
		if atIdx < 0 {
			return nil
		}
		serverPort = strings.Split(decoded[atIdx+1:], ":")
		decoded = decoded[:atIdx]
	}
	if len(serverPort) < 2 {
		return nil
	}
	colonIdx := strings.Index(decoded, ":")
	if colonIdx < 0 {
		return nil
	}
	return map[string]any{
		"name":     name,
		"type":     "ss",
		"server":   serverPort[0],
		"port":     strToInt(serverPort[1]),
		"cipher":   decoded[:colonIdx],
		"password": decoded[colonIdx+1:],
	}
}

func parseVmess(body, name string) map[string]any {
	dec, err := base64Decode(body)
	if err != nil {
		return nil
	}
	var raw map[string]any
	if json.Unmarshal([]byte(dec), &raw) != nil {
		return nil
	}
	// vmess 名称取 JSON 的 ps 字段（mihomo 行为），fallback 到 URI # 片段
	if ps := fmt.Sprint(raw["ps"]); ps != "" && ps != "<nil>" {
		name = ps
	}
	return map[string]any{
		"name":    name,
		"type":    "vmess",
		"server":  fmt.Sprint(raw["add"]),
		"port":    strToInt(raw["port"]),
		"uuid":    fmt.Sprint(raw["id"]),
		"alterId": strToInt(raw["aid"]),
		"cipher":  strOr(raw["scy"], "auto"),
	}
}

func parseVless(body, name string) map[string]any {
	u, err := url.Parse("vless://" + body)
	if err != nil {
		return nil
	}
	q := u.Query()
	security := q.Get("security")
	p := map[string]any{
		"name":   name,
		"type":   "vless",
		"server": u.Hostname(),
		"port":   strToInt(u.Port()),
		"udp":    true,
		"tls":    security == "tls" || security == "reality",
	}
	if flow := q.Get("flow"); flow != "" {
		p["flow"] = flow
	}
	if sni := q.Get("sni"); sni != "" {
		p["servername"] = sni
	}
	if network := q.Get("type"); network != "" {
		p["network"] = network
	}
	return p
}

func parseTrojan(body, name string) map[string]any {
	u, err := url.Parse("trojan://" + body)
	if err != nil {
		return nil
	}
	return map[string]any{
		"name":     name,
		"type":     "trojan",
		"server":   u.Hostname(),
		"port":     strToInt(u.Port()),
		"password": u.User.Username(),
	}
}

func parseHysteria2(body, name string) map[string]any {
	u, err := url.Parse("hysteria2://" + body)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	q := u.Query()
	p := map[string]any{
		"name":     name,
		"type":     "hysteria2",
		"server":   u.Hostname(),
		"port":     443,
		"password": u.User.Username(),
	}
	if port := strToInt(u.Port()); port != 0 {
		p["port"] = port
	}
	if sni := q.Get("sni"); sni != "" {
		p["sni"] = sni
	}
	if q.Get("insecure") == "1" {
		p["skip-cert-verify"] = true
	}
	if obfs := q.Get("obfs"); obfs != "" {
		p["obfs"] = obfs
	}
	if obfsPwd := q.Get("obfs-password"); obfsPwd != "" {
		p["obfs-password"] = obfsPwd
	}
	return p
}

func base64Decode(s string) (string, error) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if dec, err := enc.DecodeString(s); err == nil {
			return string(dec), nil
		}
	}
	return "", fmt.Errorf("invalid base64")
}

func strToInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}

func strOr(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}
