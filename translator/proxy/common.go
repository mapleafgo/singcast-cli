package proxy

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// GetStr 取字符串值；键缺失、为 nil 或非标量时返回 ""。
// 标量（数字/布尔）会转成字符串，因为 YAML 常把 password: 123456 解析成整数。
func GetStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case int:
		return strconv.Itoa(s)
	case int64:
		return strconv.FormatInt(s, 10)
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(s)
	default:
		// 列表/映射不能用 %v 退化成 "[/]"、"map[...]" 这类垃圾串：它们非空，
		// 会被当成合法值写进输出配置（http-opts.path 惯例就是列表），
		// 且让调用方的 GetStrSlice 兜底分支永远不可达。
		return ""
	}
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

// SecondsToDuration 把秒数转成 sing-box 可解析的 duration 字符串。
// 非正数返回 "0s"。例：300 -> "5m"、3600 -> "1h"、90 -> "1m30s"。
//
// 不用 time.Duration.String()：它恒定输出全部单位（"5m0s"、"1h0m0s"），
// 而翻译结果是要给用户阅读的配置文件，省掉零值单位更清晰。
func SecondsToDuration(seconds int) string {
	if seconds <= 0 {
		return "0s"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60

	var b strings.Builder
	if h > 0 {
		fmt.Fprintf(&b, "%dh", h)
	}
	if m > 0 {
		fmt.Fprintf(&b, "%dm", m)
	}
	if s > 0 {
		fmt.Fprintf(&b, "%ds", s)
	}
	return b.String()
}

// bandwidthPattern 对齐 mihomo common/utils.StringToBps 的输入语法：
// 十进制整数 + 可选空格 + 可选 K/M/G/T 前缀 + b(bit) 或 B(Byte) + "ps"。
// 与 mihomo 的唯一差异是前缀大小写不敏感：mihomo 的正则只认大写，
// "50 mbps" 在那边静默变成 0（即不限速），这里按用户显然的意图解析。
var bandwidthPattern = regexp.MustCompile(`^(\d+)\s*([KMGTkmgt]?)([Bb])ps$`)

// ParseBandwidth 把 mihomo 的带宽字符串解析成 sing-box 的 Mbps 整数值。
// 支持 "100 Mbps"、"1Gbps"、"12 MBps"（大写 B 为字节，按 ×8 折算）等写法；
// 无单位的纯数字按 Mbps 处理，与 mihomo 一致。
// 解析失败返回 0（sing-box 语义为不限速）；不足 1 Mbps 的限速向上取整为 1，
// 因为截断成 0 会把用户的低速限制变成完全放开。
func ParseBandwidth(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// 无单位：mihomo 按 Mbps 解释
	if mbps, err := strconv.Atoi(s); err == nil {
		return mbps
	}

	m := bandwidthPattern.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	value, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil {
		return 0
	}

	bits := value
	switch strings.ToUpper(m[2]) {
	case "T":
		bits *= 1000 * 1000 * 1000 * 1000
	case "G":
		bits *= 1000 * 1000 * 1000
	case "M":
		bits *= 1000 * 1000
	case "K":
		bits *= 1000
	}
	if m[3] == "B" {
		bits *= 8
	}

	mbps := bits / 1_000_000
	if mbps == 0 && bits > 0 {
		return 1
	}
	if mbps > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(mbps)
}

// ParseServerPorts 把 mihomo 的 ports 端口跳跃写法转成 sing-box 的 server_ports。
//
// mihomo 允许逗号分隔多段、每段可以是区间 "1000-2000" 或单端口 "5000"；
// sing-box 要求每个条目都是 "start:end" 形式。因此不能简单把 "-" 换成 ":"：
// "1000-2000,3000" 会得到 "1000:2000,3000" 这种 sing-box 解析不了的条目，
// 单端口 "5000" 也必须展开成 "5000:5000"。
// 无法解析的段被跳过；全部无效时返回 nil。
func ParseServerPorts(ports string) []string {
	var result []string
	for _, seg := range strings.Split(ports, ",") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		start, end, found := strings.Cut(seg, "-")
		start = strings.TrimSpace(start)
		if !found {
			end = start
		} else {
			end = strings.TrimSpace(end)
		}
		if !isPortNumber(start) || !isPortNumber(end) {
			continue
		}
		result = append(result, start+":"+end)
	}
	return result
}

func isPortNumber(s string) bool {
	if s == "" {
		return false
	}
	n, err := strconv.Atoi(s)
	return err == nil && n > 0 && n <= 65535
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
	if minStreams := GetInt(smux, "min-streams"); minStreams > 0 {
		multiplex["min_streams"] = minStreams
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
