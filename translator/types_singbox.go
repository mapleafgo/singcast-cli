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
	// invalidHealthCheckProxyTags 记录不应参与 url-test/fallback 健康检查的伪节点标签。
	invalidHealthCheckProxyTags map[string]bool
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
	// dnsTerminalRules 是必须排在全部 DNS 规则最后的兜底规则（无匹配条件、
	// 命中一切）。单独收集而不直接进 config.DNS.Rules，是因为 sing-box DNS 规则
	// 首匹配：兜底规则若留在中间，会遮蔽其后所有更精确的规则。
	dnsTerminalRules []map[string]any
	// dnsEnabled 标记用户是否显式启用了 DNS（dns.enable: true）。为 false 时
	// translateDNS 仍输出最小 DNS 模块（仅 bootstrap server）做兜底，
	// generateDNSRules 据此跳过 DNS 路由规则生成。
	dnsEnabled bool
	// country 是翻译管线启动时一次性确定的国家代码（小写 ISO 3166-1 alpha-2）。
	// 优先取 Options.Country，否则走 IP 地理位置自动检测（Init 阶段已缓存）。
	// 后续所有路由/DNS 规则生成都读此字段。
	country string
}

func (t *translation) warn(msg string) {
	t.warnings = append(t.warnings, msg)
}
