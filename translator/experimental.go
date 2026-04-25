package translator

func translateExperimental(cfg *RawConfig, result *singboxConfig) {
	clashAPI := map[string]any{
		"external_controller": "127.0.0.1:9090",
		"secret":              "",
		"default_mode":        "Rule",
	}
	if cfg.ExternalController != "" {
		clashAPI["external_controller"] = cfg.ExternalController
	}
	if cfg.Secret != "" {
		clashAPI["secret"] = cfg.Secret
	}
	if cfg.Mode != "" {
		clashAPI["default_mode"] = modeMap(cfg.Mode)
	}

	cacheFile := map[string]any{
		"enabled":      cfg.Profile.StoreSelected,
		"path":         "cache.db",
		"store_fakeip": cfg.Profile.StoreFakeIP,
		"store_dns":    true,
	}

	exp := map[string]any{
		"clash_api":  clashAPI,
		"cache_file": cacheFile,
	}
	result.Experimental = exp
}

func modeMap(mode string) string {
	switch mode {
	case "rule":
		return "Rule"
	case "global":
		return "Global"
	case "direct":
		return "Direct"
	default:
		return "Rule"
	}
}
