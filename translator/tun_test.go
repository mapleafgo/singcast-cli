package translator

import "testing"

func mustStringSlice(t *testing.T, m map[string]any, key string) []string {
	t.Helper()
	s, ok := m[key].([]string)
	if !ok {
		t.Fatalf("%s is not []string: %T", key, m[key])
	}
	return s
}

func mustIntSlice(t *testing.T, m map[string]any, key string) []int {
	t.Helper()
	s, ok := m[key].([]int)
	if !ok {
		t.Fatalf("%s is not []int: %T", key, m[key])
	}
	return s
}

func TestTranslateTUNEnabled(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{
			Enable:      true,
			AutoRoute:   true,
			StrictRoute: true,
			Stack:       "system",
			MTU:         9000,
		},
	}
	tt := newTestTranslation()

	translateTUN(cfg, tt)

	if len(tt.config.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(tt.config.Inbounds))
	}

	ib := tt.config.Inbounds[0]

	if ib["type"] != "tun" {
		t.Errorf("type = %v, want tun", ib["type"])
	}
	if ib["tag"] != "tun-in" {
		t.Errorf("tag = %v, want tun-in", ib["tag"])
	}
	if ib["auto_route"] != true {
		t.Errorf("auto_route = %v, want true", ib["auto_route"])
	}
	if ib["strict_route"] != true {
		t.Errorf("strict_route = %v, want true", ib["strict_route"])
	}
	if ib["stack"] != "system" {
		t.Errorf("stack = %v, want system", ib["stack"])
	}
	if ib["mtu"] != uint32(9000) {
		t.Errorf("mtu = %v, want 9000", ib["mtu"])
	}

	addresses := mustStringSlice(t, ib, "address")
	if len(addresses) != 1 {
		t.Fatalf("expected 1 address (IPv4 only), got %d", len(addresses))
	}
	if addresses[0] != "172.18.0.1/30" {
		t.Errorf("address[0] = %v, want 172.18.0.1/30", addresses[0])
	}
}

func TestTranslateTUNDisabled(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{
			Enable: false,
		},
	}
	tt := newTestTranslation()

	translateTUN(cfg, tt)

	if len(tt.config.Inbounds) != 0 {
		t.Errorf("expected 0 inbounds, got %d", len(tt.config.Inbounds))
	}
}

func TestTranslateTUNIPv6Default(t *testing.T) {
	cfg := &RawConfig{
		IPv6: true,
		Tun: RawTun{
			Enable: true,
		},
	}
	tt := newTestTranslation()

	translateTUN(cfg, tt)

	ib := tt.config.Inbounds[0]
	addresses := mustStringSlice(t, ib, "address")
	if len(addresses) != 2 {
		t.Fatalf("expected 2 addresses (v4+v6), got %d", len(addresses))
	}
	if addresses[0] != "172.18.0.1/30" {
		t.Errorf("address[0] = %v, want 172.18.0.1/30", addresses[0])
	}
	if addresses[1] != "fdfe:dcba:9876::1/126" {
		t.Errorf("address[1] = %v, want fdfe:dcba:9876::1/126", addresses[1])
	}
}

func TestTranslateTUNIPv6Disabled(t *testing.T) {
	cfg := &RawConfig{
		IPv6: false,
		Tun: RawTun{
			Enable: true,
		},
	}
	tt := newTestTranslation()

	translateTUN(cfg, tt)

	ib := tt.config.Inbounds[0]
	addresses := mustStringSlice(t, ib, "address")
	if len(addresses) != 1 {
		t.Fatalf("expected 1 address (IPv4 only), got %d", len(addresses))
	}
	if addresses[0] != "172.18.0.1/30" {
		t.Errorf("address[0] = %v, want 172.18.0.1/30", addresses[0])
	}
}

func TestTranslateTUNCustom(t *testing.T) {
	cfg := &RawConfig{
		IPv6: true,
		Tun: RawTun{
			Enable:       true,
			Inet4Address: "10.0.0.1/24",
			Inet6Address: "fd00::1/64",
			Device:       "utun0",
		},
	}
	tt := newTestTranslation()

	translateTUN(cfg, tt)

	if len(tt.config.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(tt.config.Inbounds))
	}

	ib := tt.config.Inbounds[0]

	addresses := mustStringSlice(t, ib, "address")
	if len(addresses) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addresses))
	}
	if addresses[0] != "10.0.0.1/24" {
		t.Errorf("address[0] = %v, want 10.0.0.1/24", addresses[0])
	}
	if addresses[1] != "fd00::1/64" {
		t.Errorf("address[1] = %v, want fd00::1/64", addresses[1])
	}
	if ib["interface_name"] != "utun0" {
		t.Errorf("interface_name = %v, want utun0", ib["interface_name"])
	}
}

func TestTranslateTUNRouteAddress(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{
			Enable:              true,
			RouteAddress:        []string{"0.0.0.0/1", "128.0.0.0/1"},
			RouteExcludeAddress: []string{"192.168.0.0/16"},
		},
	}
	tt := newTestTranslation()
	translateTUN(cfg, tt)

	ib := tt.config.Inbounds[0]

	ra := mustStringSlice(t, ib, "route_address")
	if len(ra) != 2 || ra[0] != "0.0.0.0/1" || ra[1] != "128.0.0.0/1" {
		t.Errorf("route_address = %v, want [0.0.0.0/1 128.0.0.0/1]", ra)
	}

	rea := mustStringSlice(t, ib, "route_exclude_address")
	if len(rea) != 1 || rea[0] != "192.168.0.0/16" {
		t.Errorf("route_exclude_address = %v, want [192.168.0.0/16]", rea)
	}
}

func TestTranslateTUNLinuxFields(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{
			Enable:             true,
			AutoRedirect:       true,
			IPRoute2TableIndex: 2022,
			IPRoute2RuleIndex:  9000,
			IncludeUID:         []int{0, 1000},
			IncludeUIDRange:    []string{"1000:9999"},
			ExcludeUID:         []int{105},
			ExcludeUIDRange:    []string{"0:999"},
		},
	}
	tt := newTestTranslation()
	translateTUN(cfg, tt)

	ib := tt.config.Inbounds[0]

	if ib["auto_redirect"] != true {
		t.Errorf("auto_redirect = %v, want true", ib["auto_redirect"])
	}
	if ib["iproute2_table_index"] != 2022 {
		t.Errorf("iproute2_table_index = %v, want 2022", ib["iproute2_table_index"])
	}
	if ib["iproute2_rule_index"] != 9000 {
		t.Errorf("iproute2_rule_index = %v, want 9000", ib["iproute2_rule_index"])
	}

	iu := mustIntSlice(t, ib, "include_uid")
	if len(iu) != 2 || iu[0] != 0 || iu[1] != 1000 {
		t.Errorf("include_uid = %v, want [0 1000]", iu)
	}

	iur := mustStringSlice(t, ib, "include_uid_range")
	if len(iur) != 1 || iur[0] != "1000:9999" {
		t.Errorf("include_uid_range = %v, want [1000:9999]", iur)
	}

	eu := mustIntSlice(t, ib, "exclude_uid")
	if len(eu) != 1 || eu[0] != 105 {
		t.Errorf("exclude_uid = %v, want [105]", eu)
	}

	eur := mustStringSlice(t, ib, "exclude_uid_range")
	if len(eur) != 1 || eur[0] != "0:999" {
		t.Errorf("exclude_uid_range = %v, want [0:999]", eur)
	}
}

func TestTranslateTUNAndroidFields(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{
			Enable:             true,
			IncludeAndroidUser: []int{0, 10},
			IncludePackage:     []string{"com.android.chrome"},
			ExcludePackage:     []string{"com.android.captiveportallogin"},
		},
	}
	tt := newTestTranslation()
	translateTUN(cfg, tt)

	ib := tt.config.Inbounds[0]

	iau := mustIntSlice(t, ib, "include_android_user")
	if len(iau) != 2 || iau[0] != 0 || iau[1] != 10 {
		t.Errorf("include_android_user = %v, want [0 10]", iau)
	}

	ip := mustStringSlice(t, ib, "include_package")
	if len(ip) != 1 || ip[0] != "com.android.chrome" {
		t.Errorf("include_package = %v, want [com.android.chrome]", ip)
	}

	ep := mustStringSlice(t, ib, "exclude_package")
	if len(ep) != 1 || ep[0] != "com.android.captiveportallogin" {
		t.Errorf("exclude_package = %v, want [com.android.captiveportallogin]", ep)
	}
}

func TestTranslateTUNNoPlatform(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{
			Enable: true,
		},
	}
	tt := newTestTranslation()
	translateTUN(cfg, tt)

	ib := tt.config.Inbounds[0]
	if _, exists := ib["platform"]; exists {
		t.Error("platform should not be set (sing-box TunPlatformOptions only supports http_proxy)")
	}
}

func TestTranslateTUNAutoDetectInterface(t *testing.T) {
	falseVal := false
	cfg := &RawConfig{
		Tun: RawTun{
			Enable:              true,
			AutoDetectInterface: &falseVal,
		},
	}
	tt := newTestTranslation()
	translateGeneral(cfg, tt.config)
	translateTUN(cfg, tt)

	if tt.config.Route.AutoDetectInterface != false {
		t.Errorf("auto_detect_interface = %v, want false", tt.config.Route.AutoDetectInterface)
	}
}

func TestTranslateTUNAutoDetectInterfaceDefault(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{
			Enable: true,
		},
	}
	tt := newTestTranslation()
	translateGeneral(cfg, tt.config)
	translateTUN(cfg, tt)

	if tt.config.Route.AutoDetectInterface != true {
		t.Errorf("auto_detect_interface = %v, want true (default from general)", tt.config.Route.AutoDetectInterface)
	}
}

func TestTranslateTUNDefaultStack(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{
			Enable: true,
		},
	}
	tt := newTestTranslation()
	translateTUN(cfg, tt)

	ib := tt.config.Inbounds[0]
	if ib["stack"] != "mixed" {
		t.Errorf("stack = %v, want mixed", ib["stack"])
	}
}

func TestTranslateTUNUDPTimeout(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{Enable: true, UDPTimeout: 300},
	}
	tt := newTestTranslation()
	translateTUN(cfg, tt)

	ib := tt.config.Inbounds[0]
	timeout, ok := ib["udp_timeout"].(string)
	if !ok {
		t.Fatalf("udp_timeout is not string: %T", ib["udp_timeout"])
	}
	if timeout != "5m" {
		t.Errorf("udp_timeout = %v, want 5m", timeout)
	}
}

func TestTranslateTUNUDPTimeoutZero(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{Enable: true, UDPTimeout: 0},
	}
	tt := newTestTranslation()
	translateTUN(cfg, tt)

	if _, exists := tt.config.Inbounds[0]["udp_timeout"]; exists {
		t.Errorf("udp_timeout should not be set when 0")
	}
}

func TestTranslateTUNEmptySliceOmitted(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{
			Enable:              true,
			RouteAddress:        []string{},
			RouteExcludeAddress: []string{},
			IncludeUID:          []int{},
			IncludePackage:      []string{},
		},
	}
	tt := newTestTranslation()
	translateTUN(cfg, tt)

	ib := tt.config.Inbounds[0]
	for _, key := range []string{"route_address", "route_exclude_address", "include_uid", "include_package"} {
		if _, exists := ib[key]; exists {
			t.Errorf("%s should not be set for empty slice", key)
		}
	}
}

func TestTranslateTUNAutoRouteFalse(t *testing.T) {
	cfg := &RawConfig{
		Tun: RawTun{Enable: true, AutoRoute: false, StrictRoute: false},
	}
	tt := newTestTranslation()
	translateTUN(cfg, tt)

	ib := tt.config.Inbounds[0]
	// auto_route and strict_route are always written (even false)
	if ib["auto_route"] != false {
		t.Errorf("auto_route = %v, want false", ib["auto_route"])
	}
	if ib["strict_route"] != false {
		t.Errorf("strict_route = %v, want false", ib["strict_route"])
	}
}
