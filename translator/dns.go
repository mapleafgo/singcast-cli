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

	// Build DNS server entries from various mihomo DNS sections
	defaultServerTags := buildDNSServerEntries(dns.DefaultNameserver, "def-", "", result)
	nameserverTags := buildDNSServerEntries(dns.NameServer, "ns-", "", result)
	fallbackTags := buildDNSServerEntries(dns.Fallback, "fb-", detour, result)
	psnTags := buildDNSServerEntries(dns.ProxyServerNameserver, "psn-", "", result)

	// Determine best domain_resolver: prefer proxy-server-nameserver, fallback to default-nameserver
	domainResolverTags := defaultServerTags
	if len(psnTags) > 0 {
		domainResolverTags = psnTags
	}
	_ = buildDNSServerEntries(dns.DirectNameserver, "dn-", "", result)

	// Step 5: Domain resolver chain — for servers with domain-based server addresses,
	// set domain_resolver pointing to a default-nameserver (IP-based UDP resolver).
	for _, srv := range result.Servers {
		serverAddr, _ := srv["server"].(string)
		if serverAddr == "" {
			continue
		}
		if !isIPAddress(serverAddr) {
			// This server's address is a domain; it needs a domain_resolver.
			// Pick the first default-nameserver that is IP-based.
			if len(domainResolverTags) > 0 {
				srv["domain_resolver"] = domainResolverTags[0]
			} else {
				t.warn("DNS server \"" + serverAddr + "\" has a domain address but no default-nameserver is configured; this may cause DNS resolution deadlock")
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
			"type":       "fakeip",
			"tag":        fakeipTag,
			"inet4_range": inet4Range,
		}
		if dns.FakeIPRange6 != "" {
			fakeipSrv["inet6_range"] = normalizeFakeIPRange6(dns.FakeIPRange6)
		}
		result.Servers = append(result.Servers, fakeipSrv)

		// Add DNS rules for fake-ip-filter domains → route to non-fakeip server.
		if len(dns.FakeIPFilter) > 0 && firstNSTag != "" {
			var suffixes []string
			var domains []string

			for _, f := range dns.FakeIPFilter {
				if strings.HasPrefix(f, "*.") {
					suffixes = append(suffixes, f[1:]) // "*.lan" -> ".lan"
				} else if strings.Contains(f, "*") {
					// Wildcard patterns that are not simple suffix
					// Convert to keyword or suffix as best effort
					suffixes = append(suffixes, "."+strings.TrimPrefix(f, "*"))
				} else {
					domains = append(domains, f)
				}
			}

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

	// Step 6b: Hosts mapping (mihomo hosts -> sing-box hosts DNS server + rules)
	if len(cfg.Hosts) > 0 {
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

	// Step 7: DNS rules from nameserver-policy
	for pattern, serverVal := range dns.NameServerPolicy {
		// Value can be a single URL string or a list of URLs
		urls := policyToURLs(serverVal)
		if len(urls) == 0 {
			continue
		}

		policyTag := "pol-" + strconv.Itoa(len(result.Servers))
		if len(urls) > 1 {
			t.warn("nameserver-policy \"" + pattern + "\" has " + strconv.Itoa(len(urls)) + " URLs, only the first is used")
		}
		srv := parseDNSServer(urls[0], policyTag, "")
		if srv == nil {
			continue
		}
		// Handle domain_resolver for policy servers with domain addresses
		if serverAddr, _ := srv["server"].(string); serverAddr != "" && !isIPAddress(serverAddr) {
			if len(domainResolverTags) > 0 {
				srv["domain_resolver"] = domainResolverTags[0]
			}
		}
		result.Servers = append(result.Servers, srv)

		rule := nameserverPolicyToRule(pattern, policyTag)
		if rule != nil {
			result.Rules = append(result.Rules, rule)
		}
	}

	// Step 8: DNS rules from fallback-filter
	if len(fallbackTags) > 0 {
		fbTag := fallbackTags[0]
		ff := dns.FallbackFilter

		// geosite rules → route to fallback
		for _, gs := range ff.GeoSite {
			rsName := "geosite-" + gs
			result.Rules = append(result.Rules, map[string]any{
				"rule_set": []string{rsName},
				"server":   fbTag,
			})
			ensureRuleSetDef(rsName, "geosite", gs, t)
		}

		// geoip rule → non-CN IPs use fallback
		if ff.GeoIP {
			geoCode := ff.GeoIPCode
			if geoCode == "" {
				geoCode = "cn"
			}
			rsName := "geoip-" + geoCode
			result.Rules = append(result.Rules, map[string]any{
				"rule_set": []string{rsName},
				"invert":   true,
				"server":   fbTag,
			})
			ensureRuleSetDef(rsName, "geoip", geoCode, t)
		}

		// domain list in fallback-filter → route to fallback
		for _, d := range ff.Domain {
			result.Rules = append(result.Rules, map[string]any{
				"domain": []string{d},
				"server": fbTag,
			})
		}

		// ipcidr rules in fallback-filter → route to fallback
		for _, cidr := range ff.IPCIDR {
			result.Rules = append(result.Rules, map[string]any{
				"ip_cidr": []string{cidr},
				"server":  fbTag,
			})
		}
	}

	// Step 9: Final DNS server
	if fakeIPEnabled && fakeipTag != "" {
		result.Final = fakeipTag
	} else if len(fallbackTags) > 0 {
		result.Final = fallbackTags[0]
	} else if firstNSTag != "" {
		result.Final = firstNSTag
	}

	// Add action:"route" to all DNS rules with a server field (sing-box v1.11+ convention)
	for _, rule := range result.Rules {
		if _, hasAction := rule["action"]; !hasAction {
			if _, hasServer := rule["server"]; hasServer {
				rule["action"] = "route"
			}
		}
	}

	t.config.DNS = result
}

// parseDNSServer parses a mihomo DNS URL string into a sing-box DNS server object.
// defaultDetour is the proxy group tag to use for detour if "#proxy" is in params.
func parseDNSServer(rawURL string, tag string, defaultDetour string) map[string]any {
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
	dnsType := schemeToDNSType(scheme, params)
	srv["type"] = dnsType

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

	// Process # parameters
	if useProxy, ok := params["proxy"]; ok && useProxy == "" {
		// "#proxy" means use detour
		if defaultDetour != "" {
			srv["detour"] = defaultDetour
		}
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
func schemeToDNSType(scheme string, params map[string]string) string {
	// Check for h3 override parameter
	if _, ok := params["h3"]; ok {
		return "h3"
	}

	switch strings.ToLower(scheme) {
	case "https", "http":
		// sing-box has no plain HTTP DNS type; http:// is treated as https://
		return "https"
	case "tls":
		return "tls"
	case "quic":
		// sing-box has no quic DNS type; quic:// is treated as h3
		return "h3"
	case "h3":
		return "h3"
	default:
		// Plain IP or host:port → UDP
		return "udp"
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

// firstGroupTag returns the first group tag in insertion order,
// or empty string if no groups are available.
func firstGroupTag(t *translation) string {
	if len(t.groupTagOrder) == 0 {
		return ""
	}
	return t.groupTagOrder[0]
}

// nameserverPolicyToRule converts a mihomo nameserver-policy pattern to a sing-box DNS rule.
func nameserverPolicyToRule(pattern string, serverTag string) map[string]any {
	// mihomo patterns:
	//   "+.example.com" → domain suffix ".example.com"
	//   "example.com"   → domain keyword "example.com" (exact match → domain)
	//   "*.example.com" → domain suffix ".example.com"

	if strings.HasPrefix(pattern, "+.") {
		// "+.example.com" → domain_suffix [".example.com"]
		return map[string]any{
			"domain_suffix": []string{pattern[1:]}, // strip leading "+"
			"server":        serverTag,
		}
	}

	if strings.HasPrefix(pattern, "*.") {
		// "*.example.com" → domain_suffix [".example.com"]
		return map[string]any{
			"domain_suffix": []string{pattern[1:]}, // strip leading "*"
			"server":        serverTag,
		}
	}

	if strings.Contains(pattern, "*") {
		// Contains wildcard but not at the start — use domain_keyword as best effort
		return map[string]any{
			"domain_keyword": []string{strings.ReplaceAll(pattern, "*", "")},
			"server":         serverTag,
		}
	}

	// Plain domain name → exact match
	return map[string]any{
		"domain": []string{pattern},
		"server": serverTag,
	}
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

func buildDNSServerEntries(servers []string, prefix string, detour string, result *singboxDNS) []string {
	var tags []string
	for i, ns := range servers {
		tag := prefix + strconv.Itoa(i)
		srv := parseDNSServer(ns, tag, detour)
		if srv != nil {
			result.Servers = append(result.Servers, srv)
			tags = append(tags, tag)
		}
	}
	return tags
}

// policyToURLs extracts DNS server URL strings from a nameserver-policy value.
// The value can be a single string or a list of strings.
func policyToURLs(val any) []string {
	switch v := val.(type) {
	case string:
		return []string{v}
	case []any:
		var urls []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				urls = append(urls, s)
			}
		}
		return urls
	default:
		return nil
	}
}
