package translator

// RawConfig 是 mihomo YAML 的宽松输入结构。未知字段由 YAML 解析器忽略，
// 已知但暂不支持的字段由各翻译阶段通过 warnings 告知调用方。
type RawConfig struct {
	Port       int `yaml:"port" json:"port"`
	SocksPort  int `yaml:"socks-port" json:"socks-port"`
	RedirPort  int `yaml:"redir-port" json:"redir-port"`
	TProxyPort int `yaml:"tproxy-port" json:"tproxy-port"`
	MixedPort  int `yaml:"mixed-port" json:"mixed-port"`
	// Extension: not a standard mihomo field. Translates to sing-box mixed inbound set_system_proxy.
	MixedSystemProxy   bool                      `yaml:"mixed-system-proxy" json:"mixed-system-proxy"`
	Authentication     []string                  `yaml:"authentication" json:"authentication"`
	AllowLan           bool                      `yaml:"allow-lan" json:"allow-lan"`
	BindAddress        string                    `yaml:"bind-address" json:"bind-address"`
	Mode               string                    `yaml:"mode" json:"mode"`
	LogLevel           string                    `yaml:"log-level" json:"log-level"`
	IPv6               bool                      `yaml:"ipv6" json:"ipv6"`
	ExternalController string                    `yaml:"external-controller" json:"external-controller"`
	ExternalUI         string                    `yaml:"external-ui" json:"external-ui"`
	Secret             string                    `yaml:"secret" json:"secret"`
	Interface          string                    `yaml:"interface-name" json:"interface-name"`
	RoutingMark        int                       `yaml:"routing-mark" json:"routing-mark"`
	Sniffer            RawSniffer                `yaml:"sniffer" json:"sniffer"`
	DNS                RawDNS                    `yaml:"dns" json:"dns"`
	Tun                RawTun                    `yaml:"tun" json:"tun"`
	Profile            RawProfile                `yaml:"profile" json:"profile"`
	Proxy              []map[string]any          `yaml:"proxies" json:"proxies"`
	ProxyGroup         []map[string]any          `yaml:"proxy-groups" json:"proxy-groups"`
	Rule               []string                  `yaml:"rules" json:"rules"`
	ProxyProvider      map[string]map[string]any `yaml:"proxy-providers" json:"proxy-providers"`
	RuleProvider       map[string]map[string]any `yaml:"rule-providers" json:"rule-providers"`
	Hosts              map[string]any            `yaml:"hosts" json:"hosts"`
	KeepAliveInterval  int                       `yaml:"keep-alive-interval" json:"keep-alive-interval"`
	FindProcessMode    string                    `yaml:"find-process-mode" json:"find-process-mode"`
	GlobalFingerprint  string                    `yaml:"global-client-fingerprint" json:"global-client-fingerprint"`
}

// RawSniffer 保存 mihomo sniffer 输入；当前翻译策略固定启用 sing-box sniff，
// 这些字段只用于保持输入结构完整。
type RawSniffer struct {
	Enable      bool `yaml:"enable" json:"enable"`
	Parsing     any  `yaml:"parsing" json:"parsing"`
	Sniff       any  `yaml:"sniff" json:"sniff"`
	SkipDest    any  `yaml:"skip-dest" json:"skip-dest"`
	Force       any  `yaml:"force" json:"force"`
	ForceDns    any  `yaml:"force-dns-mapping" json:"force-dns-mapping"`
	ParsePureIP any  `yaml:"parse-pure-ip" json:"parse-pure-ip"`
}

// RawDNS 保存 mihomo DNS 配置，字段语义以 mihomo 文档为准。
type RawDNS struct {
	Enable                bool              `yaml:"enable" json:"enable"`
	IPv6                  *bool             `yaml:"ipv6" json:"ipv6"`
	UseHosts              *bool             `yaml:"use-hosts" json:"use-hosts"`
	NameServer            []string          `yaml:"nameserver" json:"nameserver"`
	Fallback              []string          `yaml:"fallback" json:"fallback"`
	FallbackFilter        RawFallbackFilter `yaml:"fallback-filter" json:"fallback-filter"`
	Listen                string            `yaml:"listen" json:"listen"`
	EnhancedMode          string            `yaml:"enhanced-mode" json:"enhanced-mode"`
	FakeIPRange           string            `yaml:"fake-ip-range" json:"fake-ip-range"`
	FakeIPRange6          string            `yaml:"fake-ip-range6" json:"fake-ip-range6"`
	FakeIPFilter          []string          `yaml:"fake-ip-filter" json:"fake-ip-filter"`
	FakeIPFilterMode      string            `yaml:"fake-ip-filter-mode" json:"fake-ip-filter-mode"`
	DefaultNameserver     []string          `yaml:"default-nameserver" json:"default-nameserver"`
	NameServerPolicy      map[string]any    `yaml:"nameserver-policy" json:"nameserver-policy"`
	ProxyServerNameserver []string          `yaml:"proxy-server-nameserver" json:"proxy-server-nameserver"`
	DirectNameserver      []string          `yaml:"direct-nameserver" json:"direct-nameserver"`
	PreferH3              bool              `yaml:"prefer-h3" json:"prefer-h3"`
}

// RawFallbackFilter 描述 fallback 结果的地理/IP 过滤条件。
type RawFallbackFilter struct {
	GeoIP     bool     `yaml:"geoip" json:"geoip"`
	GeoIPCode string   `yaml:"geoip-code" json:"geoip-code"`
	IPCIDR    []string `yaml:"ipcidr" json:"ipcidr"`
	Domain    []string `yaml:"domain" json:"domain"`
	GeoSite   []string `yaml:"geosite" json:"geosite"`
}

// RawProfile 保存 mihomo profile 持久化选项。
type RawProfile struct {
	StoreSelected bool `yaml:"store-selected" json:"store-selected"`
	StoreFakeIP   bool `yaml:"store-fake-ip" json:"store-fake-ip"`
}

// RawTun 保存 mihomo TUN 输入；端口、UID、包名等平台字段按原语义映射。
type RawTun struct {
	Enable              bool     `yaml:"enable" json:"enable"`
	Device              string   `yaml:"device" json:"device"`
	Stack               string   `yaml:"stack" json:"stack"`
	DNSHijack           []string `yaml:"dns-hijack" json:"dns-hijack"`
	AutoRoute           bool     `yaml:"auto-route" json:"auto-route"`
	AutoDetectInterface *bool    `yaml:"auto-detect-interface" json:"auto-detect-interface"`
	AutoRedirect        bool     `yaml:"auto-redirect" json:"auto-redirect"`
	MTU                 uint32   `yaml:"mtu" json:"mtu"`
	Inet4Address        string   `yaml:"inet4-address" json:"inet4-address"`
	Inet6Address        string   `yaml:"inet6-address" json:"inet6-address"`
	StrictRoute         bool     `yaml:"strict-route" json:"strict-route"`
	UDPTimeout          int64    `yaml:"udp-timeout" json:"udp-timeout"`
	RouteAddress        []string `yaml:"route-address" json:"route-address"`
	RouteExcludeAddress []string `yaml:"route-exclude-address" json:"route-exclude-address"`
	IPRoute2TableIndex  int      `yaml:"iproute2-table-index" json:"iproute2-table-index"`
	IPRoute2RuleIndex   int      `yaml:"iproute2-rule-index" json:"iproute2-rule-index"`
	IncludeUID          []int    `yaml:"include-uid" json:"include-uid"`
	IncludeUIDRange     []string `yaml:"include-uid-range" json:"include-uid-range"`
	ExcludeUID          []int    `yaml:"exclude-uid" json:"exclude-uid"`
	ExcludeUIDRange     []string `yaml:"exclude-uid-range" json:"exclude-uid-range"`
	IncludeAndroidUser  []int    `yaml:"include-android-user" json:"include-android-user"`
	IncludePackage      []string `yaml:"include-package" json:"include-package"`
	ExcludePackage      []string `yaml:"exclude-package" json:"exclude-package"`
}
