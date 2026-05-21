package translator

import (
	"net"
	"strconv"
	"strings"
)

// translateDNS converts mihomo DNS configuration to sing-box DNS configuration.
// This is the most complex translator due to the fundamentally different architectures:
// mihomo uses nameserver+fallback+fallback-filter+nameserver-policy (layered model),
// sing-box uses servers+rules (routing model).
//
// Note: mihomo's nameserver-policy is NOT translated here. Instead, the equivalent
// functionality is independently implemented in autoroute.go's generateDNSRules(),
// which generates geo-based DNS routing rules (domestic → direct DNS, foreign → fallback)
// using sing-box's rule_set mechanism. The RawConfig.NameServerPolicy field is parsed
// but intentionally ignored — see autoroute.go for the implementation.
func translateDNS(cfg *RawConfig, t *translation) {
	dns := cfg.DNS
	if !dns.Enable {
		return
	}

	result := &singboxDNS{
		Servers: []map[string]any{},
		Rules:   []map[string]any{},
	}

	// Step 1: Strategy mapping
	if dns.IPv6 != nil {
		if *dns.IPv6 {
			result.Strategy = "prefer_ipv6"
		} else {
			result.Strategy = "prefer_ipv4"
		}
	}

	// Determine first proxy group tag for detour fields.
	detour := firstGroupTag(t)

	// Build DNS server entries from various mihomo DNS sections.
	// default-nameserver and proxy-server-nameserver use plain/China DNS — no detour needed.
	// nameserver and fallback may use foreign DoH (Cloudflare, Google) — set detour so they
	// route through the proxy, ensuring they work in GFW environments.
	warn := t.warn
	defaultServerTags := buildDNSServerEntries(dns.DefaultNameserver, "def-", "", result, warn)
	nameserverTags := buildDNSServerEntries(dns.NameServer, "ns-", detour, result, warn)
	fallbackTags := buildDNSServerEntries(dns.Fallback, "fb-", detour, result, warn)
	psnTags := buildDNSServerEntries(dns.ProxyServerNameserver, "psn-", "", result, warn)

	// Determine best domain_resolver: prefer proxy-server-nameserver, fallback to default-nameserver
	domainResolverTags := defaultServerTags
	if len(psnTags) > 0 {
		domainResolverTags = psnTags
	}

	// Set route.default_domain_resolver: used by sing-box to resolve outbound (proxy) server domains.
	// This is the official sing-box mechanism for the DNS chicken-and-egg problem.
	// Prefer a plain UDP DNS server (IP-based, no domain to resolve itself).
	if len(defaultServerTags) > 0 {
		tag := preferUDPServer(defaultServerTags, result)
		t.config.Route.DefaultDomainResolver = tag
	} else {
		t.warn("no default-nameserver configured; proxy server domains may not resolve correctly")
	}

	// Step 5: Domain resolver chain — for servers with domain-based server addresses,
	// set domain_resolver pointing to a default-nameserver (IP-based UDP resolver).
	// Avoid circular dependency: skip servers whose tag matches the domain_resolver.
	for _, srv := range result.Servers {
		serverAddr, _ := srv["server"].(string)
		if serverAddr == "" {
			continue
		}
		if !isIPAddress(serverAddr) {
			srvTag, _ := srv["tag"].(string)
			if len(domainResolverTags) > 0 {
				for _, drTag := range domainResolverTags {
					if drTag != srvTag {
						srv["domain_resolver"] = drTag
						break
					}
				}
			}
			if _, hasDR := srv["domain_resolver"]; !hasDR && len(defaultServerTags) > 0 {
				for _, dsTag := range defaultServerTags {
					if dsTag != srvTag {
						srv["domain_resolver"] = dsTag
						break
					}
				}
			}
			if _, hasDR := srv["domain_resolver"]; !hasDR {
				t.warn("DNS server \"" + serverAddr + "\" has a domain address but no non-circular domain_resolver is available")
			}
		}
	}

	// Determine the first nameserver tag for use as final and fakeip-filter target.
	firstNSTag := ""
	if len(nameserverTags) > 0 {
		firstNSTag = nameserverTags[0]
	}

	// Step 6: FakeIP handling
	fakeIPEnabled := dns.EnhancedMode == "fake-ip"
	var fakeipTag string

	if fakeIPEnabled {
		fakeipTag = "fakeip-dns"
		inet4Range := "198.18.0.0/15"
		if dns.FakeIPRange != "" {
			inet4Range = normalizeFakeIPRange(dns.FakeIPRange)
		}

		fakeipSrv := map[string]any{
			"type":        "fakeip",
			"tag":         fakeipTag,
			"inet4_range": inet4Range,
		}
		if dns.FakeIPRange6 != "" {
			fakeipSrv["inet6_range"] = normalizeFakeIPRange6(dns.FakeIPRange6)
		}
		result.Servers = append(result.Servers, fakeipSrv)

		// FakeIP filter rules
		if len(dns.FakeIPFilter) > 0 && firstNSTag != "" {
			var suffixes []string
			var domains []string

			for _, f := range dns.FakeIPFilter {
				if strings.HasPrefix(f, "*.") {
					suffixes = append(suffixes, f[1:]) // "*.lan" -> ".lan"
				} else if strings.Contains(f, "*") {
					suffixes = append(suffixes, "."+strings.TrimPrefix(f, "*"))
				} else {
					domains = append(domains, f)
				}
			}

			if dns.FakeIPFilterMode == "whitelist" {
				// Whitelist: only matched domains use FakeIP, rest use nameserver
				if len(suffixes) > 0 {
					result.Rules = append(result.Rules, map[string]any{
						"domain_suffix": suffixes,
						"server":        fakeipTag,
					})
				}
				if len(domains) > 0 {
					result.Rules = append(result.Rules, map[string]any{
						"domain": domains,
						"server": fakeipTag,
					})
				}
				// Default all other domains to non-fakeip nameserver
				result.Rules = append(result.Rules, map[string]any{
					"server": firstNSTag,
				})
			} else {
				// Blacklist (default): matched domains bypass FakeIP → nameserver
				if len(suffixes) > 0 {
					result.Rules = append(result.Rules, map[string]any{
						"domain_suffix": suffixes,
						"server":        firstNSTag,
					})
				}
				if len(domains) > 0 {
					result.Rules = append(result.Rules, map[string]any{
						"domain": domains,
						"server": firstNSTag,
					})
				}
			}
		}
	}

	// Step 6b: Hosts mapping (mihomo hosts -> sing-box hosts DNS server + rules)
	if len(cfg.Hosts) > 0 && (dns.UseHosts == nil || *dns.UseHosts) {
		hostsTag := "hosts"
		predefined := make(map[string]any)
		var domains []string
		for domain, ip := range cfg.Hosts {
			predefined[domain] = ip
			domains = append(domains, domain)
		}
		result.Servers = append(result.Servers, map[string]any{
			"type":       "hosts",
			"tag":        hostsTag,
			"predefined": predefined,
		})
		if len(domains) > 0 {
			result.Rules = append(result.Rules, map[string]any{
				"domain": domains,
				"server": hostsTag,
			})
		}
	}

		// Step 7: DNS rules generated by generateDNSRules (called after translateDNS).

	// Step 9: Final DNS server
	// With route.default_domain_resolver set, proxy server domains resolve via that
	// dedicated server, so DNS final can safely use the user's intended nameserver/fallback.
	if len(fallbackTags) > 0 {
		result.Final = fallbackTags[0]
	} else if firstNSTag != "" {
		result.Final = firstNSTag
	}

	// Global prefer-h3: upgrade all HTTPS DNS servers to H3
	if dns.PreferH3 {
		for _, srv := range result.Servers {
			if srv["type"] == "https" {
				srv["type"] = "h3"
			}
		}
	}

	t.config.DNS = result
}

// parseDNSServer parses a mihomo DNS URL string into a sing-box DNS server object.
// defaultDetour is the proxy group tag to use for detour if "#proxy" is in params.
func parseDNSServer(rawURL string, tag string, defaultDetour string, warn func(string)) map[string]any {
	s := strings.TrimSpace(rawURL)
	if s == "" {
		return nil
	}

	// Handle special cases first
	switch {
	case s == "system":
		return map[string]any{
			"type": "local",
			"tag":  tag,
		}
	case s == "fakeip":
		return map[string]any{
			"type":        "fakeip",
			"tag":         tag,
			"inet4_range": "198.18.0.0/15",
		}
	case strings.HasPrefix(s, "rcode://"):
		rcode := strings.TrimPrefix(s, "rcode://")
		return map[string]any{
			"type":  "rcode",
			"tag":   tag,
			"rcode": rcode,
		}
	case strings.HasPrefix(s, "dhcp://"):
		iface := strings.TrimPrefix(s, "dhcp://")
		return map[string]any{
			"type":      "dhcp",
			"tag":       tag,
			"interface": iface,
		}
	}

	// Parse URL with optional # parameters
	host, port, path, scheme, params := extractHostPort(s)

	srv := map[string]any{
		"tag": tag,
	}

	// Determine server type
	dnsType, degraded := schemeToDNSType(scheme, params)
	srv["type"] = dnsType
	if degraded {
		warn("DNS server \"" + host + "\": sing-box has no plain HTTP DNS type, \"http://\" degraded to \"https\"")
	}

	// Set server address
	srv["server"] = host

	// Set server port
	if port > 0 {
		srv["server_port"] = port
	}

	// Set path for HTTPS/h3
	if path != "" && (dnsType == "https" || dnsType == "h3") {
		srv["path"] = path
	}

	// Set detour if provided (for nameserver/fallback servers that need proxy access)
	if defaultDetour != "" {
		srv["detour"] = defaultDetour
	}
	if _, ok := params["skip-cert-verify"]; ok {
		if dnsType == "https" || dnsType == "tls" || dnsType == "h3" {
			srv["tls"] = map[string]any{
				"enabled":  true,
				"insecure": true,
			}
		}
	}
	if ecsAddr, ok := params["ecs"]; ok {
		srv["client_subnet"] = ecsAddr
	}
	if _, ok := params["disable-ipv4"]; ok {
		srv["strategy"] = "ipv6_only"
	}
	if _, ok := params["disable-ipv6"]; ok {
		srv["strategy"] = "ipv4_only"
	}

	return srv
}

// schemeToDNSType maps URL scheme to sing-box DNS server type.
// Returns the type and true if the mapping was a degradation.
func schemeToDNSType(scheme string, params map[string]string) (string, bool) {
	// Check for h3 override parameter (#h3 or #h3=true enables, #h3=false disables)
	if h3Val, ok := params["h3"]; ok && h3Val != "false" {
		return "h3", false
	}

	switch strings.ToLower(scheme) {
	case "https":
		return "https", false
	case "http":
		// sing-box has no plain HTTP DNS type; degrade to https
		return "https", true
	case "tls":
		return "tls", false
	case "quic":
		return "quic", false
	case "h3":
		return "h3", false
	default:
		// Plain IP or host:port → UDP
		return "udp", false
	}
}

// extractHostPort parses host, port, path and scheme from a mihomo DNS URL string.
// It handles mihomo's "#param&param" fragment syntax for additional parameters.
func extractHostPort(rawURL string) (host string, port int, path string, scheme string, params map[string]string) {
	params = make(map[string]string)
	s := rawURL

	// Split off fragment (#params)
	if before, after, ok := strings.Cut(s, "#"); ok {
		s = before
		// Parse fragment as key&key=value pairs
		for _, part := range strings.Split(after, "&") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if key, value, ok := strings.Cut(part, "="); ok {
				params[key] = value
			} else {
				params[part] = ""
			}
		}
	}

	// Determine scheme
	scheme = ""
	if strings.HasPrefix(s, "https://") {
		scheme = "https"
		s = s[len("https://"):]
	} else if strings.HasPrefix(s, "http://") {
		scheme = "http"
		s = s[len("http://"):]
	} else if strings.HasPrefix(s, "tls://") {
		scheme = "tls"
		s = s[len("tls://"):]
	} else if strings.HasPrefix(s, "quic://") {
		scheme = "quic"
		s = s[len("quic://"):]
	} else if strings.HasPrefix(s, "h3://") {
		scheme = "h3"
		s = s[len("h3://"):]
	}

	// Extract path
	path = ""
	if scheme == "https" || scheme == "http" || scheme == "h3" {
		if slashIdx := strings.Index(s, "/"); slashIdx >= 0 {
			path = s[slashIdx:]
			s = s[:slashIdx]
		}
	}

	// Extract host and port
	host = s
	port = 0

	// Handle [ipv6]:port
	if strings.HasPrefix(host, "[") {
		if closeBracket := strings.Index(host, "]"); closeBracket >= 0 {
			hostPart := host[1:closeBracket]
			rest := host[closeBracket+1:]
			if strings.HasPrefix(rest, ":") {
				portStr := rest[1:]
				if p, err := strconv.Atoi(portStr); err == nil {
					port = p
				}
			}
			host = hostPart
		}
	} else {
		// host:port
		if colonIdx := strings.LastIndex(host, ":"); colonIdx >= 0 {
			portStr := host[colonIdx+1:]
			if p, err := strconv.Atoi(portStr); err == nil {
				port = p
				host = host[:colonIdx]
			}
		}
	}

	// Default ports based on scheme
	if port == 0 {
		switch scheme {
		case "https", "http", "h3":
			port = 443
		case "tls", "quic":
			port = 853
		default:
			port = 53
		}
	}

	return host, port, path, scheme, params
}

// isIPAddress checks if a string is a valid IP address (v4 or v6).
func isIPAddress(s string) bool {
	return net.ParseIP(s) != nil
}

// addDNSRouteAction adds action:"route" to all DNS rules with a server field.
// Must run after generateDNSRules so all rules are present.
func addDNSRouteAction(t *translation) {
	if t.config.DNS == nil {
		return
	}
	for _, rule := range t.config.DNS.Rules {
		if _, hasAction := rule["action"]; !hasAction {
			if _, hasServer := rule["server"]; hasServer {
				rule["action"] = "route"
			}
		}
	}
}

// firstGroupTag returns the first group tag in insertion order,
// or empty string if no groups are available.
func firstGroupTag(t *translation) string {
	if len(t.groupTagOrder) == 0 {
		return ""
	}
	return t.groupTagOrder[0]
}

// findFirstDirectDNSTag returns the tag of the first DNS server without a detour
// and not of type fakeip/hosts. Used as the "domestic DNS" target.
func findFirstDirectDNSTag(t *translation) string {
	if t.config.DNS == nil {
		return ""
	}
	for _, srv := range t.config.DNS.Servers {
		tp, _ := srv["type"].(string)
		if tp == "fakeip" || tp == "hosts" {
			continue
		}
		if _, hasDetour := srv["detour"]; hasDetour {
			continue
		}
		if tag, _ := srv["tag"].(string); tag != "" {
			return tag
		}
	}
	return ""
}

// findFakeIPTag returns the tag of the fakeip DNS server, or empty if not present.
func findFakeIPTag(t *translation) string {
	if t.config.DNS == nil {
		return ""
	}
	for _, srv := range t.config.DNS.Servers {
		if srv["type"] == "fakeip" {
			if tag, _ := srv["tag"].(string); tag != "" {
				return tag
			}
		}
	}
	return ""
}

func detectCC(t *translation) string {
	if t.opts != nil && t.opts.Country != "" {
		if cc := strings.ToLower(strings.TrimSpace(t.opts.Country)); len(cc) == 2 {
			return cc
		}
	}
	return strings.ToLower(DetectCountry(""))
}

var fakeIPV4Net = mustParseCIDR("198.18.0.0/15")

func mustParseCIDR(s string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return ipNet
}

func normalizeFakeIPRange(ipRange string) string {
	_, ipNet, err := net.ParseCIDR(ipRange)
	if err != nil {
		return "198.18.0.0/15"
	}
	// If the range falls within the standard fake-ip range, normalize to /15
	if fakeIPV4Net.Contains(ipNet.IP) {
		return "198.18.0.0/15"
	}
	return ipNet.String()
}

// normalizeFakeIPRange6 converts mihomo fake-ip-range6 to a CIDR suitable for sing-box.
func normalizeFakeIPRange6(ipRange string) string {
	_, ipNet, err := net.ParseCIDR(ipRange)
	if err != nil {
		return ipRange
	}
	return ipNet.String()
}

func buildDNSServerEntries(servers []string, prefix string, detour string, result *singboxDNS, warn func(string)) []string {
	var tags []string
	for i, ns := range servers {
		tag := prefix + strconv.Itoa(i)
		srv := parseDNSServer(ns, tag, detour, warn)
		if srv != nil {
			result.Servers = append(result.Servers, srv)
			tags = append(tags, tag)
		}
	}
	return tags
}

// preferUDPServer returns the tag of the first UDP-type DNS server from candidates,
// falling back to the first candidate if none is UDP.
func preferUDPServer(candidates []string, result *singboxDNS) string {
	if len(candidates) == 0 {
		return ""
	}
	for _, tag := range candidates {
		for _, srv := range result.Servers {
			if srv["tag"] == tag && srv["type"] == "udp" {
				return tag
			}
		}
	}
	return candidates[0]
}
