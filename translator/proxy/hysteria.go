package proxy

import "strings"

// TranslateHysteria translates a mihomo Hysteria (v1) proxy config to a sing-box outbound.
func TranslateHysteria(m map[string]any, warn func(string)) map[string]any {
	outbound := map[string]any{
		"type": "hysteria",
		"tag":  GetStr(m, "name"),
	}
	ApplyCommonFields(m, outbound)

	// Auth
	if auth := GetStr(m, "auth-str"); auth != "" {
		outbound["auth_str"] = auth
	} else if auth := GetStr(m, "auth"); auth != "" {
		outbound["auth_str"] = auth
	}

	// Bandwidth (up/down)
	if up := GetStr(m, "up"); up != "" {
		if mbps := ParseBandwidth(up); mbps > 0 {
			outbound["up_mbps"] = mbps
		}
	} else if upSpeed := GetInt(m, "up-speed"); upSpeed > 0 {
		outbound["up_mbps"] = upSpeed
	}
	if down := GetStr(m, "down"); down != "" {
		if mbps := ParseBandwidth(down); mbps > 0 {
			outbound["down_mbps"] = mbps
		}
	} else if downSpeed := GetInt(m, "down-speed"); downSpeed > 0 {
		outbound["down_mbps"] = downSpeed
	}

	// Obfs
	if obfs := GetStr(m, "obfs"); obfs != "" {
		outbound["obfs"] = obfs
	}

	// Server ports (port hopping)
	if ports := GetStr(m, "ports"); ports != "" {
		outbound["server_ports"] = []string{strings.ReplaceAll(ports, "-", ":")}
	}

	// Hop interval
	if hopInterval := GetInt(m, "hop-interval"); hopInterval > 0 {
		outbound["hop_interval"] = SecondsToDuration(hopInterval)
	}

	// Receive windows (deprecated but still accepted)
	if recvConn := GetInt(m, "recv-window-conn"); recvConn > 0 {
		outbound["recv_window_conn"] = recvConn
	}
	if recv := GetInt(m, "recv-window"); recv > 0 {
		outbound["recv_window"] = recv
	}
	if GetBool(m, "disable-mtu-discovery") {
		outbound["disable_mtu_discovery"] = true
	}

	// Network (protocol: udp/tcp/...)
	if proto := GetStr(m, "protocol"); proto != "" {
		outbound["network"] = proto
	}

	// TLS
	tls := BuildAlwaysOnTLS(m)
	if fingerprint := GetStr(m, "fingerprint"); fingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
	}
	outbound["tls"] = tls

	return outbound
}
