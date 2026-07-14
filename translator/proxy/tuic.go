package proxy

import "github.com/gofrs/uuid/v5"

var supportedTUICUDPRelayModes = map[string]bool{
	"native": true,
	"quic":   true,
}

var supportedTUICCongestionControllers = map[string]bool{
	"cubic":    true,
	"new_reno": true,
	"bbr":      true,
}

// TranslateTUIC translates a mihomo TUIC proxy config to a sing-box outbound.
// Only TUIC v5 (uuid + password) is supported; TUIC v4 (token) is not.
// TLS is always enabled for TUIC.
// See mapping doc section B.11.
func TranslateTUIC(m map[string]any, warn func(string)) map[string]any {
	// Check for TUIC v4 (token-based auth) — not supported
	if token := GetStr(m, "token"); token != "" {
		warn("tuic: TUIC v4 (token authentication) is not supported by sing-box, skipping")
		return nil
	}

	outbound := map[string]any{
		"type": "tuic",
		"tag":  GetStr(m, "name"),
	}
	ApplyCommonFields(m, outbound)

	// UUID (required for v5)
	if uuidValue := GetStr(m, "uuid"); uuidValue != "" {
		if _, err := uuid.FromString(uuidValue); err != nil {
			warn("tuic: invalid uuid, skipping")
			return nil
		}
		outbound["uuid"] = uuidValue
	} else {
		warn("tuic: missing uuid, skipping")
		return nil
	}

	// Password (required for v5)
	if password := GetStr(m, "password"); password != "" {
		outbound["password"] = password
	} else {
		warn("tuic: missing password, skipping")
		return nil
	}

	// UDP relay mode
	if udpRelayMode := GetStr(m, "udp-relay-mode"); udpRelayMode != "" {
		if !supportedTUICUDPRelayModes[udpRelayMode] {
			warn("tuic: udp-relay-mode '" + udpRelayMode + "' is not supported by sing-box, skipping")
			return nil
		}
		outbound["udp_relay_mode"] = udpRelayMode
	}

	// Congestion controller -> congestion_control
	if cc := GetStr(m, "congestion-controller"); cc != "" {
		if !supportedTUICCongestionControllers[cc] {
			warn("tuic: congestion-controller '" + cc + "' is not supported by sing-box, skipping")
			return nil
		}
		outbound["congestion_control"] = cc
	}

	// Reduce RTT -> zero_rtt_handshake
	if reduceRTT := GetBool(m, "reduce-rtt"); reduceRTT {
		outbound["zero_rtt_handshake"] = true
	}

	tls := BuildAlwaysOnTLS(m)
	// ALPN is required for TUIC; set default if not provided
	if _, hasALPN := tls["alpn"]; !hasALPN {
		tls["alpn"] = []string{"h3"}
	}
	outbound["tls"] = tls

	return outbound
}
