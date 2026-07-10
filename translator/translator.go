package translator

import (
	"encoding/json"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Options controls translation behavior.
type Options struct {
	Country          string // override auto-detected country code (e.g. "US", "JP")
	RuleSetURLPrefix string // URL prefix for rule_set downloads (e.g. "https://gh-proxy.org"), empty = direct
}

// Translate translates a mihomo YAML config to a sing-box JSON config string.
// Returns the JSON string, a list of warnings, and any fatal error.
func Translate(data []byte) (string, []string, error) {
	return TranslateWithOptions(data, nil)
}

// Meta holds post-translation metadata that callers (e.g. core.Service) may need.
type Meta struct {
	// StubTags maps outbound tag → original protocol for proxies that were
	// converted to socks stubs because the protocol is unsupported by sing-box.
	StubTags map[string]string
}

// TranslateWithMeta is like TranslateWithOptions but also returns translation metadata.
func TranslateWithMeta(data []byte, opts *Options) (string, []string, Meta, error) {
	jsonStr, warnings, meta, err := translateInternal(data, opts)
	return jsonStr, warnings, meta, err
}

// TranslateWithOptions translates with additional configuration options.
func TranslateWithOptions(data []byte, opts *Options) (string, []string, error) {
	jsonStr, warnings, _, err := translateInternal(data, opts)
	return jsonStr, warnings, err
}

func translateInternal(data []byte, opts *Options) (string, []string, Meta, error) {
	// Detect format
	if DetectFormat(data) == FormatJSON {
		// Already sing-box JSON, pass through
		return string(data), nil, Meta{}, nil
	}

	// Parse YAML
	cfg := &RawConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return "", nil, Meta{}, fmt.Errorf("parse YAML: %w", err)
	}

	t := &translation{
		config: &singboxConfig{
			Inbounds:  []map[string]any{},
			Outbounds: []map[string]any{},
			Route:     &singboxRoute{},
		},
		proxyTags:   make(map[string]bool),
		groupTags:   make(map[string]bool),
		stubTags:    make(map[string]string),
		ruleSetDefs: make(map[string]map[string]any),
		opts:        opts,
	}

	// Step 1-2: Global config → inbounds + log
	translateGeneral(cfg, t.config)

	// Step 3: Translate proxies → outbounds
	proxyOutbounds := translateProxies(cfg, t)

	// Step 3b: Collect ECH query-server-name domains from translated outbounds.
	// These need direct DNS routing to avoid circular dependency:
	// proxy → ECH config fetch → DNS (via proxy) → proxy loop.
	collectECHQueryServers(proxyOutbounds, t)

	// Step 4: Translate proxy groups → outbounds
	groupOutbounds := translateGroups(cfg, t)

	// Combine outbounds: proxies first, then groups
	t.config.Outbounds = append(t.config.Outbounds, proxyOutbounds...)
	t.config.Outbounds = append(t.config.Outbounds, groupOutbounds...)

	// Step 5: Auto-routing (serenity-style, based on detected country)
	translateRules(cfg, t)

	// Step 6: Translate DNS
	translateDNS(cfg, t)

	// Step 6b: Generate DNS rules based on detected country (needs DNS servers from Step 6)
	generateDNSRules(detectCC(t), t)

	// Step 6c: Add action:"route" to all DNS rules with a server field
	addDNSRouteAction(t)

	// Step 7: Translate TUN
	translateTUN(cfg, t)

	// Step 8: Translate experimental
	translateExperimental(cfg, t.config)

	// Step 9: Assemble (inject builtins, default rules, convert REJECT, rule_set defs)
	assemble(t)

	// Step 10: Validate (after assemble so REJECT is already converted to action)
	if err := validate(t); err != nil {
		return "", t.warnings, Meta{StubTags: t.stubTags}, err
	}

	// Serialize to JSON
	jsonBytes, err := json.MarshalIndent(t.config, "", "  ")
	if err != nil {
		return "", t.warnings, Meta{StubTags: t.stubTags}, fmt.Errorf("marshal JSON: %w", err)
	}

	return string(jsonBytes), t.warnings, Meta{StubTags: t.stubTags}, nil
}
