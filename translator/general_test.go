package translator

import "testing"

func TestTranslateGeneralMixedPort(t *testing.T) {
	cfg := &RawConfig{MixedPort: 7890}
	result := &singboxConfig{}

	translateGeneral(cfg, result)

	if len(result.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(result.Inbounds))
	}
	ib := result.Inbounds[0]
	if ib["type"] != "mixed" {
		t.Errorf("inbound type = %v, want mixed", ib["type"])
	}
	if ib["tag"] != "mixed-in" {
		t.Errorf("inbound tag = %v, want mixed-in", ib["tag"])
	}
	if ib["listen_port"] != 7890 {
		t.Errorf("listen_port = %v, want 7890", ib["listen_port"])
	}
	if ib["listen"] != "127.0.0.1" {
		t.Errorf("listen = %v, want 127.0.0.1", ib["listen"])
	}
}

func TestTranslateGeneralAllowLan(t *testing.T) {
	tests := []struct {
		name     string
		allowLan bool
		want     string
	}{
		{"allow-lan true", true, "0.0.0.0"},
		{"allow-lan false", false, "127.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RawConfig{MixedPort: 7890, AllowLan: tt.allowLan}
			result := &singboxConfig{}

			translateGeneral(cfg, result)

			if len(result.Inbounds) == 0 {
				t.Fatal("expected at least 1 inbound")
			}
			got := result.Inbounds[0]["listen"].(string)
			if got != tt.want {
				t.Errorf("listen = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTranslateGeneralLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"warning maps to warn", "warning", "warn"},
		{"silent maps to error", "silent", "error"},
		{"empty defaults to info", "", "info"},
		{"info passes through", "info", "info"},
		{"debug passes through", "debug", "debug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RawConfig{LogLevel: tt.input}
			result := &singboxConfig{}

			translateGeneral(cfg, result)

			if result.Log == nil {
				t.Fatal("log config is nil")
			}
			got := result.Log["level"].(string)
			if got != tt.want {
				t.Errorf("log.level = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTranslateGeneralMultiplePorts(t *testing.T) {
	cfg := &RawConfig{
		Port:      8080,
		SocksPort: 1080,
		MixedPort: 7890,
	}
	result := &singboxConfig{}

	translateGeneral(cfg, result)

	if len(result.Inbounds) != 3 {
		t.Fatalf("expected 3 inbounds, got %d", len(result.Inbounds))
	}

	wantTypes := []struct {
		typ  string
		tag  string
		port int
	}{
		{"mixed", "mixed-in", 7890},
		{"http", "http-in", 8080},
		{"socks", "socks-in", 1080},
	}

	for i, w := range wantTypes {
		ib := result.Inbounds[i]
		if ib["type"] != w.typ {
			t.Errorf("inbound[%d].type = %v, want %v", i, ib["type"], w.typ)
		}
		if ib["tag"] != w.tag {
			t.Errorf("inbound[%d].tag = %v, want %v", i, ib["tag"], w.tag)
		}
		if ib["listen_port"] != w.port {
			t.Errorf("inbound[%d].listen_port = %v, want %v", i, ib["listen_port"], w.port)
		}
	}
}

func TestTranslateGeneralFindProcess(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want bool
	}{
		{"always enables find_process", "always", true},
		{"strict enables find_process", "strict", true},
		{"off disables find_process", "off", false},
		{"empty disables find_process", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RawConfig{FindProcessMode: tt.mode}
			result := &singboxConfig{}

			translateGeneral(cfg, result)

			if result.Route == nil {
				t.Fatal("route is nil")
			}
			if result.Route.FindProcess != tt.want {
				t.Errorf("FindProcess = %v, want %v", result.Route.FindProcess, tt.want)
			}
		})
	}
}

func TestTranslateGeneralInterface(t *testing.T) {
	cfg := &RawConfig{
		Interface:   "eth0",
		RoutingMark: 1234,
	}
	result := &singboxConfig{}

	translateGeneral(cfg, result)

	if result.Route == nil {
		t.Fatal("route is nil")
	}
	if result.Route.DefaultInterface != "eth0" {
		t.Errorf("DefaultInterface = %v, want eth0", result.Route.DefaultInterface)
	}
	if result.Route.DefaultMark != 1234 {
		t.Errorf("DefaultMark = %v, want 1234", result.Route.DefaultMark)
	}
}
