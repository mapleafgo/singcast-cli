package proxy

// TranslateHTTP translates a mihomo HTTP proxy config to a sing-box outbound.
// See mapping doc section B.12.
func TranslateHTTP(m map[string]any, warn func(string)) map[string]any {
	outbound := map[string]any{
		"type": "http",
		"tag":  GetStr(m, "name"),
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

	// Headers
	if headers := GetMap(m, "headers"); headers != nil {
		outbound["headers"] = headers
	}

	// TLS if enabled
	if tls := TranslateTLS(m, warn); tls != nil {
		outbound["tls"] = tls
	}

	return outbound
}
