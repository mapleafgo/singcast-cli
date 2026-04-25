package translator

type singboxConfig struct {
	Log          map[string]any   `json:"log,omitempty"`
	DNS          *singboxDNS      `json:"dns,omitempty"`
	Inbounds     []map[string]any `json:"inbounds"`
	Outbounds    []map[string]any `json:"outbounds"`
	Route        *singboxRoute    `json:"route"`
	Experimental map[string]any   `json:"experimental,omitempty"`
}

type singboxRoute struct {
	Rules                []map[string]any  `json:"rules"`
	RuleSet              []map[string]any  `json:"rule_set,omitempty"`
	Final                string            `json:"final"`
	AutoDetectInterface  bool              `json:"auto_detect_interface"`
	FindProcess          bool              `json:"find_process,omitempty"`
	DefaultInterface     string            `json:"default_interface,omitempty"`
	DefaultMark          int               `json:"default_mark,omitempty"`
	DefaultDomainResolver string           `json:"default_domain_resolver,omitempty"`
}

type singboxDNS struct {
	Servers []map[string]any `json:"servers"`
	Rules   []map[string]any `json:"rules,omitempty"`
	Final   string           `json:"final"`
	Strategy string          `json:"strategy,omitempty"`
}

type translation struct {
	config   *singboxConfig
	warnings []string
	// translated proxy tags for reference validation
	proxyTags map[string]bool
	// translated group tags (insertion-ordered via slice + set)
	groupTagOrder []string
	groupTags     map[string]bool
	// rule_set definitions accumulated from GEOIP/GEOSITE rules
	ruleSetDefs map[string]map[string]any
}

func (t *translation) warn(msg string) {
	t.warnings = append(t.warnings, msg)
}
