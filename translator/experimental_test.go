package translator

import "testing"

func TestTranslateExperimentalDefaults(t *testing.T) {
	cfg := &RawConfig{}
	result := &singboxConfig{}

	translateExperimental(cfg, result)

	if result.Experimental == nil {
		t.Fatal("experimental is nil")
	}

	clashAPI, ok := result.Experimental["clash_api"].(map[string]any)
	if !ok {
		t.Fatal("clash_api is not a map")
	}

	if clashAPI["external_controller"] != "127.0.0.1:9090" {
		t.Errorf("external_controller = %v, want 127.0.0.1:9090", clashAPI["external_controller"])
	}
	if clashAPI["secret"] != "" {
		t.Errorf("secret = %v, want empty string", clashAPI["secret"])
	}
	if clashAPI["default_mode"] != "Rule" {
		t.Errorf("default_mode = %v, want Rule", clashAPI["default_mode"])
	}
}

func TestTranslateExperimentalCustom(t *testing.T) {
	tests := []struct {
		name               string
		externalController string
		secret             string
		mode               string
		wantMode           string
	}{
		{"custom controller", "0.0.0.0:9091", "", "", "Rule"},
		{"custom secret", "127.0.0.1:9090", "my-secret", "", "Rule"},
		{"mode rule", "", "", "rule", "Rule"},
		{"mode global", "", "", "global", "Global"},
		{"mode direct", "", "", "direct", "Direct"},
		{"mode unknown defaults to Rule", "", "", "unknown", "Rule"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RawConfig{
				ExternalController: tt.externalController,
				Secret:             tt.secret,
				Mode:               tt.mode,
			}
			result := &singboxConfig{}

			translateExperimental(cfg, result)

			clashAPI := result.Experimental["clash_api"].(map[string]any)

			wantController := tt.externalController
			if wantController == "" {
				wantController = "127.0.0.1:9090"
			}
			if clashAPI["external_controller"] != wantController {
				t.Errorf("external_controller = %v, want %v", clashAPI["external_controller"], wantController)
			}

			if clashAPI["secret"] != tt.secret {
				t.Errorf("secret = %v, want %v", clashAPI["secret"], tt.secret)
			}

			if clashAPI["default_mode"] != tt.wantMode {
				t.Errorf("default_mode = %v, want %v", clashAPI["default_mode"], tt.wantMode)
			}
		})
	}
}

func TestTranslateExperimentalCacheFile(t *testing.T) {
	tests := []struct {
		name          string
		storeSelected bool
		storeFakeIP   bool
	}{
		{"both disabled", false, false},
		{"store-selected only", true, false},
		{"store-fake-ip only", false, true},
		{"both enabled", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RawConfig{
				Profile: RawProfile{
					StoreSelected: tt.storeSelected,
					StoreFakeIP:   tt.storeFakeIP,
				},
			}
			result := &singboxConfig{}

			translateExperimental(cfg, result)

			cacheFile := result.Experimental["cache_file"].(map[string]any)

			if cacheFile["enabled"] != tt.storeSelected {
				t.Errorf("cache_file.enabled = %v, want %v", cacheFile["enabled"], tt.storeSelected)
			}
			if cacheFile["store_fakeip"] != tt.storeFakeIP {
				t.Errorf("cache_file.store_fakeip = %v, want %v", cacheFile["store_fakeip"], tt.storeFakeIP)
			}
			if cacheFile["path"] != "cache.db" {
				t.Errorf("cache_file.path = %v, want cache.db", cacheFile["path"])
			}
			if cacheFile["store_dns"] != true {
				t.Error("cache_file.store_dns should always be true")
			}
		})
	}
}
