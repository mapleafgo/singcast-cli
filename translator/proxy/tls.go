package proxy

// TranslateTLS translates mihomo TLS configuration to a sing-box tls object.
// Returns nil if no TLS is needed (tls field is not truthy and no reality-opts
// or other TLS-related fields are present).
//
// Sing-box OutboundTLSOptions fields mapped from Mihomo:
//
//	enabled                       <- tls
//	server_name                   <- sni / servername
//	insecure                      <- skip-cert-verify
//	alpn                          <- alpn
//	certificate_public_key_sha256 <- fingerprint
//	client_certificate            <- certificate
//	client_key                    <- private-key
//	utls                          <- client-fingerprint
//	reality                       <- reality-opts
//	ech                           <- ech-opts
//
// Sing-box OutboundTLSOptions fields (Mihomo has no corresponding field):
//
//	disable_sni, min_version, max_version, cipher_suites,
//	curve_preferences, certificate_path, client_certificate_path,
//	client_key_path, fragment, fragment_fallback_delay, record_fragment
func TranslateTLS(m map[string]any) map[string]any {
	tlsEnabled := GetBool(m, "tls")
	realityOpts := GetMap(m, "reality-opts")
	echOpts := GetMap(m, "ech-opts")
	sni := GetStr(m, "sni")
	servername := GetStr(m, "servername")
	skipCertVerify := GetBool(m, "skip-cert-verify")
	alpn := GetStrSlice(m, "alpn")
	fingerprint := GetStr(m, "fingerprint")
	cert := GetStr(m, "certificate")
	privateKey := GetStr(m, "private-key")
	clientFingerprint := GetStr(m, "client-fingerprint")

	// If nothing TLS-related is configured, return nil.
	if !tlsEnabled && realityOpts == nil && echOpts == nil &&
		sni == "" && servername == "" && !skipCertVerify &&
		alpn == nil && fingerprint == "" && cert == "" &&
		privateKey == "" && clientFingerprint == "" {
		return nil
	}

	tls := make(map[string]any)
	tls["enabled"] = true

	// SNI: mihomo uses both "sni" (trojan) and "servername" (vmess/vless)
	serverName := sni
	if serverName == "" {
		serverName = servername
	}
	if serverName != "" {
		tls["server_name"] = serverName
	}

	if skipCertVerify {
		tls["insecure"] = true
	}

	if alpn != nil {
		tls["alpn"] = alpn
	}

	// uTLS fingerprint (section K.2)
	applyUTLS(m, tls)

	// REALITY (section K.3)
	if realityOpts != nil {
		reality := map[string]any{"enabled": true}
		if pk := GetStr(realityOpts, "public-key"); pk != "" {
			reality["public_key"] = pk
		}
		if sid := GetStr(realityOpts, "short-id"); sid != "" {
			reality["short_id"] = sid
		}
		tls["reality"] = reality
	}

	// Certificate fingerprint SHA256 -> array
	if fingerprint != "" {
		tls["certificate_public_key_sha256"] = []string{fingerprint}
	}

	// mTLS certificate and key -> arrays
	if cert != "" {
		tls["client_certificate"] = []string{cert}
	}
	if privateKey != "" {
		tls["client_key"] = []string{privateKey}
	}

	// ECH Encrypted Client Hello (section K.4)
	if echOpts != nil {
		ech := make(map[string]any)
		if GetBool(echOpts, "enable") {
			ech["enabled"] = true
		}
		if cfg := GetStr(echOpts, "config"); cfg != "" {
			ech["config"] = []string{cfg}
		}
		if qsn := GetStr(echOpts, "query-server-name"); qsn != "" {
			ech["query_server_name"] = qsn
		}
		if len(ech) > 0 {
			tls["ech"] = ech
		}
	}

	return tls
}

// applyUTLS handles the client-fingerprint -> tls.utls mapping.
// mihomo fingerprint values: chrome, firefox, safari, ios, android, edge,
// 360, qq, random, randomized.
// These map directly to sing-box tls.utls.fingerprint with utls.enabled set to true.
func applyUTLS(m map[string]any, tls map[string]any) {
	fp := GetStr(m, "client-fingerprint")
	if fp == "" {
		return
	}

	utls := map[string]any{
		"enabled":     true,
		"fingerprint": fp,
	}
	tls["utls"] = utls
}
