package translator

import "testing"

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

	addresses, ok := ib["address"].([]string)
	if !ok {
		t.Fatalf("address is not []string: %T", ib["address"])
	}
	if len(addresses) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addresses))
	}
	if addresses[0] != "172.18.0.1/30" {
		t.Errorf("address[0] = %v, want 172.18.0.1/30", addresses[0])
	}
	if addresses[1] != "fdfe:dcba:9876::1/126" {
		t.Errorf("address[1] = %v, want fdfe:dcba:9876::1/126", addresses[1])
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

func TestTranslateTUNCustom(t *testing.T) {
	cfg := &RawConfig{
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

	addresses, ok := ib["address"].([]string)
	if !ok {
		t.Fatalf("address is not []string: %T", ib["address"])
	}
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
