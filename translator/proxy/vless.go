package proxy

var supportedVLESSFlows = map[string]bool{
	"":                 true,
	"xtls-rprx-vision": true,
}

var supportedVLESSPacketEncodings = map[string]bool{
	"":           true,
	"packetaddr": true,
	"xudp":       true,
}

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
		warn("vless: missing uuid, skipping")
		return nil
	}

	if flow := GetStr(m, "flow"); flow != "" {
		if !supportedVLESSFlows[flow] {
			warn("vless: flow '" + flow + "' is not supported by sing-box, skipping")
			return nil
		}
		outbound["flow"] = flow
	}

	// Packet encoding
	if packetEnc := GetStr(m, "packet-encoding"); packetEnc != "" {
		if !supportedVLESSPacketEncodings[packetEnc] {
			warn("vless: packet-encoding '" + packetEnc + "' is not supported by sing-box, skipping")
			return nil
		}
		outbound["packet_encoding"] = packetEnc
	}

	// TLS
	if tls := TranslateTLS(m); tls != nil {
		outbound["tls"] = tls
	}

	// Transport
	if transport := TranslateTransport(m, warn); transport != nil {
		outbound["transport"] = transport
	}

	// Multiplex
	applyMultiplex(m, outbound)

	return outbound
}
