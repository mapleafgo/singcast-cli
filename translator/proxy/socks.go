package proxy

// TranslateSOCKS translates a mihomo SOCKS5 proxy config to a sing-box outbound.
// Note: sing-box type is "socks" (not "socks5").
// See mapping doc section B.13.
func TranslateSOCKS(m map[string]any, warn func(string)) map[string]any {
	outbound := map[string]any{
		"type":    "socks",
		"tag":     GetStr(m, "name"),
		"version": "5",
	}
	ApplyCommonFields(m, outbound)

	// Username
	if username := GetStr(m, "username"); username != "" {
		outbound["username"] = username
	}

	// Password
	if password := GetStr(m, "password"); password != "" {
		outbound["password"] = password
	}

	// TLS if enabled
	if tls := TranslateTLS(m); tls != nil {
		outbound["tls"] = tls
	}

	return outbound
}
