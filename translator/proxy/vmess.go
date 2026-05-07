package proxy

// TranslateVMess translates a mihomo VMess proxy config to a sing-box outbound.
// See mapping doc section B.6.
func TranslateVMess(m map[string]any, warn func(string)) map[string]any {
	outbound := map[string]any{
		"type": "vmess",
		"tag":  GetStr(m, "name"),
	}
	ApplyCommonFields(m, outbound)

	// UUID (required)
	if uuid := GetStr(m, "uuid"); uuid != "" {
		outbound["uuid"] = uuid
	} else {
		warn("vmess: missing uuid, skipping")
		return nil
	}

	// AlterId (default 0)
	alterID := GetInt(m, "alterId")
	if alterID == 0 {
		// Also try alternate key name
		alterID = GetInt(m, "alter_id")
	}
	outbound["alter_id"] = alterID

	// Security: mihomo uses "cipher", sing-box uses "security"
	// Default is "auto"
	cipher := GetStr(m, "cipher")
	if cipher == "" {
		cipher = "auto"
	}
	outbound["security"] = cipher

	// Global padding
	if gp := GetBool(m, "global-padding"); gp {
		outbound["global_padding"] = true
	}

	// Authenticated length
	if al := GetBool(m, "authenticated-length"); al {
		outbound["authenticated_length"] = true
	}

	// Packet encoding
	if packetEnc := GetStr(m, "packet-encoding"); packetEnc != "" {
		outbound["packet_encoding"] = packetEnc
	}

	// TLS
	if tls := TranslateTLS(m); tls != nil {
		outbound["tls"] = tls
	}

	// Transport
	if transport := TranslateTransport(m); transport != nil {
		outbound["transport"] = transport
	}

	// Multiplex
	applyMultiplex(m, outbound)

	return outbound
}
