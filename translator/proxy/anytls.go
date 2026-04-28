package proxy

// TranslateAnyTLS translates a mihomo anytls proxy config to a sing-box outbound.
// AnyTLS was added in sing-box 1.12.0.
func TranslateAnyTLS(m map[string]any, warn func(string)) map[string]any {
	outbound := map[string]any{
		"type": "anytls",
		"tag":  GetStr(m, "name"),
	}
	ApplyCommonFields(m, outbound)

	// Password (required)
	if password := GetStr(m, "password"); password != "" {
		outbound["password"] = password
	} else {
		warn("anytls: missing password, skipping")
		return nil
	}

	// TLS (required for anytls)
	tls := BuildAlwaysOnTLS(m)
	outbound["tls"] = tls

	return outbound
}
