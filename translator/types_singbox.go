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
	Rules                 []map[string]any `json:"rules"`
	RuleSet               []map[string]any `json:"rule_set,omitempty"`
	Final                 string           `json:"final,omitempty"`
	AutoDetectInterface   bool             `json:"auto_detect_interface,omitempty"`
	FindProcess           bool             `json:"find_process,omitempty"`
	DefaultInterface      string           `json:"default_interface,omitempty"`
	DefaultMark           uint32           `json:"default_mark,omitempty"`
	DefaultDomainResolver string           `json:"default_domain_resolver,omitempty"`
}

type singboxDNS struct {
	Servers  []map[string]any `json:"servers"`
	Rules    []map[string]any `json:"rules,omitempty"`
	Final    string           `json:"final,omitempty"`
	Strategy string           `json:"strategy,omitempty"`
}

type translation struct {
	config   *singboxConfig
	warnings []string
	// translated proxy tags for reference validation
	proxyTags map[string]bool
	// stubTags records proxy tags that were converted to socks stubs
	// because the original protocol is unsupported. Maps tag → original type.
	stubTags map[string]string
	// translated group tags (insertion-ordered via slice + set)
	groupTagOrder []string
	groupTags     map[string]bool
	// rule_set definitions accumulated from GEOIP/GEOSITE rules
	ruleSetDefs map[string]map[string]any
	// options from caller
	opts *Options
	// echQueryServers collects ECH query-server-name values from proxy configs.
	// These domains need direct DNS routing to avoid circular dependency:
	// proxy → ECH → DNS → proxy loop.
	echQueryServers []string
}

func (t *translation) warn(msg string) {
	t.warnings = append(t.warnings, msg)
}
