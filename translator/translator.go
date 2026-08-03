package translator

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Options controls translation behavior.
type Options struct {
	// Country 覆盖自动检测的国家代码（ISO 3166-1 alpha-2），仅测试使用。
	// 生产调用方传 nil 即可，翻译器自动走 IP 地理位置检测。
	Country string
}

// Convert 统一处理订阅输入：base64 解码 → 格式识别 → JSON 透传 / URI 列表直构造 / Clash YAML 翻译。
// 纯转换，不做 rule_set URL 前缀改写（由调用方通过 ApplyRuleSetProxy 后处理）。
func Convert(data []byte) (string, []string, error) {
	jsonStr, warnings, _, err := ConvertWithMeta(data, nil)
	return jsonStr, warnings, err
}

// Meta holds post-translation metadata that callers (e.g. core.Service) may need.
type Meta struct {
	// StubTags maps outbound tag → original protocol for proxies that were
	// converted to socks stubs because the protocol is unsupported by sing-box.
	StubTags map[string]string
}

// ConvertWithMeta 是 Convert 的完整版，返回翻译元数据和 warnings。
// 统一入口：base64 解码 → 格式识别 → JSON 透传 / URI 列表直构造 / Clash YAML 翻译。
func ConvertWithMeta(data []byte, opts *Options) (string, []string, Meta, error) {
	decoded, _ := decodeBase64Input(data)
	if decoded != nil {
		data = decoded
	}
	// JSON 直接透传
	if DetectFormat(data) == FormatJSON {
		return string(data), nil, Meta{}, nil
	}
	// URI 列表：直接构造 RawConfig，跳过 YAML 序列化往返
	if isProxyURIList(data) {
		cfg, err := buildRawConfigFromURIs(data)
		if err != nil {
			return "", nil, Meta{}, err
		}
		return translateFromConfig(cfg, opts)
	}
	// Clash YAML：走标准翻译
	cfg := &RawConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return "", nil, Meta{}, fmt.Errorf("parse YAML: %w", err)
	}
	return translateFromConfig(cfg, opts)
}

// translateFromConfig 运行翻译管线：RawConfig → sing-box JSON。
// 供直接持有 RawConfig 的调用方（如 URI 列表转换）使用，跳过 YAML 序列化。
func translateFromConfig(cfg *RawConfig, opts *Options) (string, []string, Meta, error) {
	t := &translation{
		config: &singboxConfig{
			Inbounds:  []map[string]any{},
			Outbounds: []map[string]any{},
			Route:     &singboxRoute{},
		},
		proxyTags:                   make(map[string]bool),
		groupTags:                   make(map[string]bool),
		stubTags:                    make(map[string]string),
		invalidHealthCheckProxyTags: make(map[string]bool),
		ruleSetDefs:                 make(map[string]map[string]any),
		opts:                        opts,
	}

	// 提前一次性确定国家代码，后续路由/DNS 规则生成统一读 t.country。
	// Options.Country 仅测试覆盖用，且必须是两位 ISO 代码；
	// 非法值回退自动检测，避免生成 geoip-xxx/domain_suffix ".xxx" 的坏规则。
	cc := ""
	if opts != nil {
		cc = strings.ToLower(strings.TrimSpace(opts.Country))
	}
	if len(cc) != 2 {
		cc = strings.ToLower(DetectCountry(""))
	}
	t.country = cc

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

	// Step 4b: 无 proxy-groups 的订阅（v2ray URI 列表）自动生成默认组
	if len(t.groupTagOrder) == 0 {
		groupOutbounds = generateDefaultGroups(t)
	}

	// Combine outbounds: proxies first, then groups
	t.config.Outbounds = append(t.config.Outbounds, proxyOutbounds...)
	t.config.Outbounds = append(t.config.Outbounds, groupOutbounds...)

	// Step 5: Auto-routing (serenity-style, based on detected country)
	translateRules(cfg, t)

	// Step 6: Translate DNS
	translateDNS(cfg, t)

	// Step 6b: Generate DNS rules based on detected country (needs DNS servers from Step 6)
	generateDNSRules(t)

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
