package proxy

// TranslateTrojan translates a mihomo Trojan proxy config to a sing-box outbound.
// See mapping doc section B.7.
func TranslateTrojan(m map[string]any, warn func(string)) map[string]any {
	if SkipUnsupportedNetwork(m, warn) {
		return nil
	}

	outbound := map[string]any{
		"type": "trojan",
		"tag":  GetStr(m, "name"),
	}
	ApplyCommonFields(m, outbound)

	// Password (required)
	if password := GetStr(m, "password"); password != "" {
		outbound["password"] = password
	} else {
		warn("trojan: missing password, skipping")
		return nil
	}

	// TLS — Trojan almost always requires TLS; mihomo auto-enables it
	tls := TranslateTLS(m, warn)
	if tls == nil {
		tls = map[string]any{"enabled": true}
	}
	outbound["tls"] = tls

	// Transport
	if transport := TranslateTransport(m, warn); transport != nil {
		outbound["transport"] = transport
	}

	// Multiplex
	applyMultiplex(m, outbound)

	return outbound
}
