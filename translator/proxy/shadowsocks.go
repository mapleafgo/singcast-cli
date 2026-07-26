package proxy

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// supportedSSCiphers lists the Shadowsocks ciphers supported by sing-box.
var supportedSSCiphers = map[string]bool{
	"aes-128-gcm":                   true,
	"aes-192-gcm":                   true,
	"aes-256-gcm":                   true,
	"chacha20-ietf-poly1305":        true,
	"xchacha20-ietf-poly1305":       true,
	"2022-blake3-aes-128-gcm":       true,
	"2022-blake3-aes-256-gcm":       true,
	"2022-blake3-chacha20-poly1305": true,
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

	// 只按 supportedSSCiphers 白名单判定：此前还维护了一张 unsupported 黑名单，
	// 但两个分支行为完全相同（warn + 跳过），只是措辞不同，白名单已足够。
	if !supportedSSCiphers[cipher] {
		warn("shadowsocks: cipher '" + cipher + "' is not supported by sing-box, skipping")
		return nil
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
	} else {
		warn("shadowsocks: missing password, skipping")
		return nil
	}

	// Plugin：名字必须映射，不能原样透传（见 sip003PluginNames）。
	plugin := GetStr(m, "plugin")
	if plugin != "" {
		mapped, ok := sip003PluginNames[plugin]
		if !ok {
			warn("shadowsocks \"" + GetStr(m, "name") + "\": plugin \"" + plugin +
				"\" is not supported by sing-box, skipping")
			return nil
		}
		outbound["plugin"] = mapped

		// Plugin opts: convert from object to SIP003 string format
		// e.g., {mode: "tls", host: "bing.com"} -> "obfs=tls;obfs-host=bing.com"
		if pluginOpts := GetMap(m, "plugin-opts"); pluginOpts != nil {
			outbound["plugin_opts"] = buildSIP003Opts(pluginOpts, mapped)
		}
	}

	// UDP over TCP
	if uot := GetBool(m, "udp-over-tcp"); uot {
		outbound["udp_over_tcp"] = true
	}

	// Multiplex
	applyMultiplex(m, outbound)

	return outbound
}

// sip003PluginNames 把 mihomo 的插件名映射到 sing-box 注册的 SIP003 插件名。
// sing-box 的注册表只有 obfs-local 和 v2ray-plugin 两项
// （transport/sip003/{obfs,v2ray}.go），传入未注册的名字会在创建 outbound 时
// 报 "plugin not found"，导致整份配置启动失败——因此不在表内的插件
// （shadow-tls、restls、gost-plugin 等）必须降级跳过而非透传。
var sip003PluginNames = map[string]string{
	"obfs":         "obfs-local",
	"obfs-local":   "obfs-local",
	"simple-obfs":  "obfs-local",
	"v2ray-plugin": "v2ray-plugin",
}

// obfsKeyMap 把 mihomo obfs 插件的 plugin-opts 键名映射到 SIP003 标准键名。
// 只对 obfs-local 适用：v2ray-plugin 的 mode 取值是 websocket/quic，
// 改写成 obfs=websocket 会产生错误的参数串。
var obfsKeyMap = map[string]string{
	"mode": "obfs",
	"host": "obfs-host",
}

// buildSIP003Opts converts a plugin-opts object to a SIP003 format string.
// e.g., {mode: "tls", host: "bing.com"} -> "obfs=tls;obfs-host=bing.com"
// plugin 为已映射的 sing-box 插件名，用于决定是否套用 obfs 的键名改写。
func buildSIP003Opts(opts map[string]any, plugin string) string {
	if len(opts) == 0 {
		return ""
	}

	parts := make([]string, 0, len(opts))
	for _, k := range slices.Sorted(maps.Keys(opts)) {
		v := opts[k]
		key := k
		if plugin == "obfs-local" {
			if mapped, ok := obfsKeyMap[k]; ok {
				key = mapped
			}
		}
		parts = append(parts, key+"="+fmt.Sprintf("%v", v))
	}
	return strings.Join(parts, ";")
}
