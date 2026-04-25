package translator

import (
	"encoding/json"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Translate translates a mihomo YAML config to a sing-box JSON config string.
// Returns the JSON string, a list of warnings, and any fatal error.
func Translate(data []byte) (string, []string, error) {
	// Detect format
	if DetectFormat(data) == FormatJSON {
		// Already sing-box JSON, pass through
		return string(data), nil, nil
	}

	// Parse YAML
	cfg := &RawConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return "", nil, fmt.Errorf("parse YAML: %w", err)
	}

	t := &translation{
		config: &singboxConfig{
			Inbounds:  []map[string]any{},
			Outbounds: []map[string]any{},
			Route:     &singboxRoute{},
		},
		proxyTags:     make(map[string]bool),
		groupTags:     make(map[string]bool),
		ruleSetDefs:   make(map[string]map[string]any),
	}

	// Step 1-2: Global config → inbounds + log
	translateGeneral(cfg, t.config)

	// Step 3: Translate proxies → outbounds
	proxyOutbounds := translateProxies(cfg, t)

	// Step 4: Translate proxy groups → outbounds
	groupOutbounds := translateGroups(cfg, t)

	// Combine outbounds: proxies first, then groups
	t.config.Outbounds = append(t.config.Outbounds, proxyOutbounds...)
	t.config.Outbounds = append(t.config.Outbounds, groupOutbounds...)

	// Step 5: Translate rules → route.rules + route.rule_set
	translateRules(cfg, t)

	// Step 6: Translate DNS
	translateDNS(cfg, t)

	// Step 7: Translate TUN
	translateTUN(cfg, t)

	// Step 8: Translate experimental
	translateExperimental(cfg, t.config)

	// Step 9: Translate providers → rule_set
	translateProviders(cfg, t)

	// Step 10: Assemble (inject builtins, default rules, convert REJECT, rule_set defs)
	assemble(t)

	// Step 11: Validate (after assemble so REJECT is already converted to action)
	if err := validate(t); err != nil {
		return "", t.warnings, err
	}

	// Serialize to JSON
	jsonBytes, err := json.MarshalIndent(t.config, "", "  ")
	if err != nil {
		return "", t.warnings, fmt.Errorf("marshal JSON: %w", err)
	}

	return string(jsonBytes), t.warnings, nil
}
