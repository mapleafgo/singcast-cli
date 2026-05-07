package proxy

import (
	"strings"
)

const maxHopInterval = 3600 // 1 hour

// TranslateHysteria2 translates a mihomo Hysteria2 proxy config to a sing-box outbound.
// TLS is always enabled for Hysteria2.
// See mapping doc section B.9.
func TranslateHysteria2(m map[string]any, warn func(string)) map[string]any {
	outbound := map[string]any{
		"type": "hysteria2",
		"tag":  GetStr(m, "name"),
	}
	ApplyCommonFields(m, outbound)

	// Password
	if password := GetStr(m, "password"); password != "" {
		outbound["password"] = password
	}

	// Upload bandwidth
	if up := GetStr(m, "up"); up != "" {
		if mbps := ParseBandwidth(up); mbps > 0 {
			outbound["up_mbps"] = mbps
		}
	}

	// Download bandwidth
	if down := GetStr(m, "down"); down != "" {
		if mbps := ParseBandwidth(down); mbps > 0 {
			outbound["down_mbps"] = mbps
		}
	}

	// Obfuscation: obfs -> {type: "salamander", password: "..."}
	obfs := GetStr(m, "obfs")
	if obfs != "" {
		obfsPassword := GetStr(m, "obfs-password")
		obfsObj := map[string]any{
			"type": obfs,
		}
		if obfsPassword != "" {
			obfsObj["password"] = obfsPassword
		}
		outbound["obfs"] = obfsObj
	}

	// Port hopping: ports -> server_ports (convert "-" to ":")
	if ports := GetStr(m, "ports"); ports != "" {
		serverPorts := strings.ReplaceAll(ports, "-", ":")
		outbound["server_ports"] = []string{serverPorts}
	}

	// Hop interval (seconds -> duration string)
	if hopInterval := GetInt(m, "hop-interval"); hopInterval > 0 {
		outbound["hop_interval"] = SecondsToDuration(min(hopInterval, maxHopInterval))
	}

	outbound["tls"] = BuildAlwaysOnTLS(m)

	return outbound
}
