package translator

import (
	"strings"
)

func translateGeneral(cfg *RawConfig, result *singboxConfig) {
	listen := "127.0.0.1"
	if cfg.AllowLan {
		listen = "0.0.0.0"
		if cfg.BindAddress != "" && cfg.BindAddress != "*" {
			listen = cfg.BindAddress
		}
	}

	if cfg.MixedPort > 0 {
		inbound := map[string]any{
			"type":        "mixed",
			"tag":         "mixed-in",
			"listen":      listen,
			"listen_port": cfg.MixedPort,
		}
		if cfg.MixedSystemProxy {
			inbound["set_system_proxy"] = true
		}
		applyInboundAuth(cfg.Authentication, inbound)
		result.Inbounds = append(result.Inbounds, inbound)
	}
	if cfg.Port > 0 {
		inbound := map[string]any{
			"type":        "http",
			"tag":         "http-in",
			"listen":      listen,
			"listen_port": cfg.Port,
		}
		applyInboundAuth(cfg.Authentication, inbound)
		result.Inbounds = append(result.Inbounds, inbound)
	}
	if cfg.SocksPort > 0 {
		inbound := map[string]any{
			"type":        "socks",
			"tag":         "socks-in",
			"listen":      listen,
			"listen_port": cfg.SocksPort,
		}
		applyInboundAuth(cfg.Authentication, inbound)
		result.Inbounds = append(result.Inbounds, inbound)
	}
	if cfg.RedirPort > 0 {
		result.Inbounds = append(result.Inbounds, map[string]any{
			"type":        "redirect",
			"tag":         "redirect-in",
			"listen_port": cfg.RedirPort,
		})
	}
	if cfg.TProxyPort > 0 {
		result.Inbounds = append(result.Inbounds, map[string]any{
			"type":        "tproxy",
			"tag":         "tproxy-in",
			"listen_port": cfg.TProxyPort,
		})
	}

	// Log
	logLevel := mapLogLevel(cfg.LogLevel)
	logConfig := map[string]any{
		"level":     logLevel,
		"timestamp": true,
	}
	result.Log = logConfig

	// Route defaults
	if result.Route == nil {
		result.Route = &singboxRoute{}
	}
	if cfg.FindProcessMode == "always" || cfg.FindProcessMode == "strict" {
		result.Route.FindProcess = true
	}
	result.Route.AutoDetectInterface = true

	// Global interface/routing-mark
	if cfg.Interface != "" {
		result.Route.DefaultInterface = cfg.Interface
	}
	if cfg.RoutingMark > 0 {
		result.Route.DefaultMark = uint32(cfg.RoutingMark)
	}
}

// applyInboundAuth parses mihomo authentication strings ("user:pass") and adds
// sing-box inbound users format to the inbound map.
func applyInboundAuth(auth []string, inbound map[string]any) {
	if len(auth) == 0 {
		return
	}
	var users []map[string]string
	for _, a := range auth {
		user, pass, ok := strings.Cut(a, ":")
		if !ok {
			continue
		}
		users = append(users, map[string]string{
			"username": user,
			"password": pass,
		})
	}
	if len(users) > 0 {
		inbound["users"] = users
	}
}

func mapLogLevel(level string) string {
	switch level {
	case "warning":
		return "warn"
	case "silent":
		return "error"
	case "":
		return "info"
	default:
		return level
	}
}
