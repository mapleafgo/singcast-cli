package proxy

import (
	"encoding/base64"
	"encoding/pem"
	"strings"
)

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
func TranslateTLS(m map[string]any, warn func(string)) map[string]any {
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

	// mihomo 的 fingerprint 是整张证书 DER 的 SHA-256（hex）；sing-box 的
	// certificate_public_key_sha256 算的是 SPKI 公钥的 SHA-256，且字段类型是
	// []byte（JSON 侧按 base64 解码）。两者哈希对象和编码都不同，硬填进去不会报错，
	// 但会开启 InsecureSkipVerify + 自定义校验，使该出站所有握手静默失败，
	// 反而绕过了本该生效的证书链校验。sing-box 无等价选项，只能丢弃并告知用户。
	if fingerprint != "" {
		warn("proxy \"" + GetStr(m, "name") + "\": fingerprint (certificate pinning) has no " +
			"equivalent in sing-box and was ignored; TLS still uses standard chain verification")
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
			// mihomo 给的是裸 base64 的 ECHConfigList，sing-box 却对该字段做
			// pem.Decode 并要求 block type 为 "ECH CONFIGS"，裸值会让实例启动失败。
			if lines := echConfigToPEMLines(cfg); lines != nil {
				ech["config"] = lines
			} else {
				warn("proxy \"" + GetStr(m, "name") + "\": ech-opts.config is not valid base64, ECH disabled")
			}
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

// echConfigToPEMLines 把 mihomo 的裸 base64 ECHConfigList 转成 sing-box 期望的
// PEM 行数组。sing-box 会把该数组用换行拼接后做 pem.Decode，并要求 block type
// 为 "ECH CONFIGS"；直接传裸 base64 会让实例启动时报 invalid ECH configs pem。
// base64 无法解码时返回 nil，由调用方降级并告警。
func echConfigToPEMLines(b64 string) []string {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil || len(raw) == 0 {
		return nil
	}
	pemText := pem.EncodeToMemory(&pem.Block{Type: "ECH CONFIGS", Bytes: raw})
	if pemText == nil {
		return nil
	}
	// pem.Decode 要求块之后没有多余内容，因此去掉尾部换行再切分。
	return strings.Split(strings.TrimRight(string(pemText), "\n"), "\n")
}
