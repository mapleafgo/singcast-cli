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
	if cfg.IPv6 {
		if cfg.Tun.Inet6Address != "" {
			addresses = append(addresses, cfg.Tun.Inet6Address)
		} else {
			addresses = append(addresses, "fdfe:dcba:9876::1/126")
		}
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

	// Optional: UDP NAT timeout (mihomo: seconds -> sing-box: duration string)
	if cfg.Tun.UDPTimeout > 0 {
		tunInbound["udp_timeout"] = proxy.SecondsToDuration(int(cfg.Tun.UDPTimeout))
	}

	// Route addresses: specify which networks go through TUN
	if len(cfg.Tun.RouteAddress) > 0 {
		tunInbound["route_address"] = cfg.Tun.RouteAddress
	}
	if len(cfg.Tun.RouteExcludeAddress) > 0 {
		tunInbound["route_exclude_address"] = cfg.Tun.RouteExcludeAddress
	}

	// Linux: iproute2 table/rule index
	if cfg.Tun.IPRoute2TableIndex > 0 {
		tunInbound["iproute2_table_index"] = cfg.Tun.IPRoute2TableIndex
	}
	if cfg.Tun.IPRoute2RuleIndex > 0 {
		tunInbound["iproute2_rule_index"] = cfg.Tun.IPRoute2RuleIndex
	}

	// Linux: UID-based traffic filtering
	if len(cfg.Tun.IncludeUID) > 0 {
		tunInbound["include_uid"] = cfg.Tun.IncludeUID
	}
	if len(cfg.Tun.IncludeUIDRange) > 0 {
		tunInbound["include_uid_range"] = cfg.Tun.IncludeUIDRange
	}
	if len(cfg.Tun.ExcludeUID) > 0 {
		tunInbound["exclude_uid"] = cfg.Tun.ExcludeUID
	}
	if len(cfg.Tun.ExcludeUIDRange) > 0 {
		tunInbound["exclude_uid_range"] = cfg.Tun.ExcludeUIDRange
	}

	// Android: user and package filtering
	if len(cfg.Tun.IncludeAndroidUser) > 0 {
		tunInbound["include_android_user"] = cfg.Tun.IncludeAndroidUser
	}
	if len(cfg.Tun.IncludePackage) > 0 {
		tunInbound["include_package"] = cfg.Tun.IncludePackage
	}
	if len(cfg.Tun.ExcludePackage) > 0 {
		tunInbound["exclude_package"] = cfg.Tun.ExcludePackage
	}

	// auto_detect_interface is set to true by translateGeneral (correct default
	// for all platforms including desktop Linux TUN). Do not override it here.
	// On mobile, service.go also force-enables it for VpnService.protect().

	// Note: dns-hijack is handled by assemble() default rule {"protocol":"dns","action":"hijack-dns"}

	t.config.Inbounds = append(t.config.Inbounds, tunInbound)
}
