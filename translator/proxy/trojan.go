package proxy

// TranslateTrojan translates a mihomo Trojan proxy config to a sing-box outbound.
// See mapping doc section B.7.
func TranslateTrojan(m map[string]any, warn func(string)) map[string]any {
	outbound := map[string]any{
		"type": "trojan",
		"tag":  GetStr(m, "name"),
	}
	ApplyCommonFields(m, outbound)

	// Password (required)
	if password := GetStr(m, "password"); password != "" {
		outbound["password"] = password
	} else {
		warn("trojan: missing password")
	}

	// TLS — Trojan almost always requires TLS; mihomo auto-enables it
	tls := TranslateTLS(m)
	if tls == nil {
		tls = map[string]any{"enabled": true}
		warn("trojan: no TLS config found, auto-enabling TLS (mihomo behavior)")
	}
	outbound["tls"] = tls

	// Transport
	if transport := TranslateTransport(m); transport != nil {
		outbound["transport"] = transport
	}

	// Multiplex
	applyMultiplex(m, outbound, warn)

	return outbound
}
