package translator

import (
	"github.com/mapleafgo/singcast/translator/proxy"
)

// translateTUN adds a TUN inbound if TUN is enabled in the mihomo config.
func translateTUN(cfg *RawConfig, t *translation) {
	if !cfg.Tun.Enable {
		return
	}

	stack := cfg.Tun.Stack
	if stack == "" {
		stack = "mixed"
	}

	tunInbound := map[string]any{
		"type":         "tun",
		"tag":          "tun-in",
		"auto_route":   cfg.Tun.AutoRoute,
		"strict_route": cfg.Tun.StrictRoute,
		"stack":        stack,
	}

	// Build address list from inet4-address / inet6-address or defaults
	var addresses []string
	if cfg.Tun.Inet4Address != "" {
		addresses = append(addresses, cfg.Tun.Inet4Address)
	} else {
		addresses = append(addresses, "172.18.0.1/30")
	}
	if cfg.Tun.Inet6Address != "" {
		addresses = append(addresses, cfg.Tun.Inet6Address)
	} else {
		addresses = append(addresses, "fdfe:dcba:9876::1/126")
	}
	tunInbound["address"] = addresses

	// Optional: interface name (mihomo: device -> sing-box: interface_name)
	if cfg.Tun.Device != "" {
		tunInbound["interface_name"] = cfg.Tun.Device
	}

	// Optional: MTU
	if cfg.Tun.MTU > 0 {
		tunInbound["mtu"] = cfg.Tun.MTU
	}

	// Optional: auto-redirect (Linux transparent proxy)
	if cfg.Tun.AutoRedirect {
		tunInbound["auto_redirect"] = true
	}

	// Optional: endpoint-independent NAT
	tunInbound["endpoint_independent_nat"] = cfg.Tun.EndpointIndependentNat

	// Optional: UDP NAT timeout (mihomo: seconds -> sing-box: duration string)
	if cfg.Tun.UDPTimeout > 0 {
		tunInbound["udp_timeout"] = proxy.SecondsToDuration(int(cfg.Tun.UDPTimeout))
	}

	t.config.Inbounds = append(t.config.Inbounds, tunInbound)
}
