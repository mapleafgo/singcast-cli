package proxy

// TranslateVLESS translates a mihomo VLESS proxy config to a sing-box outbound.
// See mapping doc section B.5.
func TranslateVLESS(m map[string]any, warn func(string)) map[string]any {
	outbound := map[string]any{
		"type": "vless",
		"tag":  GetStr(m, "name"),
	}
	ApplyCommonFields(m, outbound)

	// UUID (required)
	if uuid := GetStr(m, "uuid"); uuid != "" {
		outbound["uuid"] = uuid
	} else {
		warn("vless: missing uuid")
	}

	// Flow (e.g., xtls-rprx-vision)
	if flow := GetStr(m, "flow"); flow != "" {
		outbound["flow"] = flow
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
	applyMultiplex(m, outbound, warn)

	return outbound
}
