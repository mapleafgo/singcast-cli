package proxy

import "math"

// TranslateTransport translates mihomo transport/underlay configuration to a sing-box
// transport object. Returns nil if no transport is needed (plain TCP).
//
// Each transport function maps only fields that sing-box recognizes, so unknown
// Mihomo fields are silently dropped instead of causing DisallowUnknownFields errors.
//
// Field mappings (Mihomo kebab-case → sing-box snake_case):
//
//	network: "ws"   -> transport.type: "ws"       (WebSocket)
//	network: "http"  -> transport.type: "http"      (HTTP/2)
//	network: "h2"    -> transport.type: "http"      (H2 uses http transport)
//	network: "grpc"  -> transport.type: "grpc"      (gRPC)
//	network: "tcp"   -> nil (no transport needed)
func TranslateTransport(m map[string]any, warn func(string)) map[string]any {
	network := GetStr(m, "network")
	if network == "" || network == "tcp" {
		return nil
	}

	// Check for v2ray-http-upgrade (ws variant that becomes httpupgrade)
	wsOpts := GetMap(m, "ws-opts")
	if network == "ws" && wsOpts != nil && GetBool(wsOpts, "v2ray-http-upgrade") {
		return translateHTTPUpgrade(wsOpts)
	}

	switch network {
	case "ws":
		return translateWS(wsOpts)
	case "http":
		return translateHTTP(m)
	case "h2":
		return translateH2(m)
	case "grpc":
		return translateGRPC(m)
	case "xhttp":
		warn("xhttp (SplitHTTP) transport is not supported by sing-box, proxy will not work")
		return nil
	default:
		return nil
	}
}

// translateWS builds a WebSocket transport from ws-opts.
//
// Sing-box V2RayWebsocketOptions:
//
//	path                string
//	headers             HTTPHeader
//	max_early_data      uint32
//	early_data_header_name string
func translateWS(wsOpts map[string]any) map[string]any {
	transport := map[string]any{
		"type": "ws",
	}

	if wsOpts == nil {
		return transport
	}

	if path := GetStr(wsOpts, "path"); path != "" {
		transport["path"] = path
	}
	if headers := getHeadersMap(wsOpts, "headers"); headers != nil {
		transport["headers"] = headers
	}
	if maxEarlyData := GetInt(wsOpts, "max-early-data"); maxEarlyData > 0 {
		transport["max_early_data"] = min(maxEarlyData, math.MaxUint32)
	}
	if earlyHeader := GetStr(wsOpts, "early-data-header-name"); earlyHeader != "" {
		transport["early_data_header_name"] = earlyHeader
	}

	return transport
}

// translateHTTPUpgrade builds an HTTP Upgrade transport from ws-opts with v2ray-http-upgrade.
//
// Sing-box V2RayHTTPUpgradeOptions:
//
//	host    string
//	path    string
//	headers HTTPHeader
func translateHTTPUpgrade(wsOpts map[string]any) map[string]any {
	transport := map[string]any{
		"type": "httpupgrade",
	}

	if wsOpts == nil {
		return transport
	}

	if host := GetStr(wsOpts, "host"); host != "" {
		transport["host"] = host
	}
	if path := GetStr(wsOpts, "path"); path != "" {
		transport["path"] = path
	}
	if headers := getHeadersMap(wsOpts, "headers"); headers != nil {
		transport["headers"] = headers
	}

	return transport
}

// translateHTTP builds an HTTP transport from http-opts.
//
// Sing-box V2RayHTTPOptions:
//
//	path         string
//	method       string
//	headers      HTTPHeader
//	host         []string   (Mihomo http-opts has no host field)
//	idle_timeout Duration   (Mihomo has no corresponding field)
//	ping_timeout Duration   (Mihomo has no corresponding field)
func translateHTTP(m map[string]any) map[string]any {
	transport := map[string]any{
		"type": "http",
	}

	httpOpts := GetMap(m, "http-opts")
	if httpOpts == nil {
		return transport
	}

	if method := GetStr(httpOpts, "method"); method != "" {
		transport["method"] = method
	}

	// http-opts.path can be a string or []string; sing-box expects a single string.
	if path := GetStr(httpOpts, "path"); path != "" {
		transport["path"] = path
	} else if paths := GetStrSlice(httpOpts, "path"); paths != nil {
		if len(paths) > 0 {
			transport["path"] = paths[0]
		}
	}

	if headers := getHeadersMap(httpOpts, "headers"); headers != nil {
		transport["headers"] = headers
	}

	return transport
}

// translateH2 builds an HTTP transport from h2-opts.
//
// Sing-box V2RayHTTPOptions (same as HTTP):
//
//	host         []string
//	path         string
//	method       string
//	headers      HTTPHeader
//	idle_timeout Duration  (Mihomo has no corresponding field)
//	ping_timeout Duration  (Mihomo has no corresponding field)
func translateH2(m map[string]any) map[string]any {
	transport := map[string]any{
		"type": "http",
	}

	h2Opts := GetMap(m, "h2-opts")
	if h2Opts == nil {
		return transport
	}

	// host can be a single string or []string; sing-box expects []string.
	if host := GetStr(h2Opts, "host"); host != "" {
		transport["host"] = []string{host}
	} else if hosts := GetStrSlice(h2Opts, "host"); hosts != nil {
		transport["host"] = hosts
	}

	if path := GetStr(h2Opts, "path"); path != "" {
		transport["path"] = path
	}

	return transport
}

// translateGRPC builds a gRPC transport from grpc-opts.
//
// Sing-box V2RayGRPCOptions:
//
//	service_name           string
//	idle_timeout           Duration  (Mihomo has no corresponding field)
//	ping_timeout           Duration  (Mihomo has no corresponding field)
//	permit_without_stream  bool      (Mihomo has no corresponding field)
func translateGRPC(m map[string]any) map[string]any {
	transport := map[string]any{
		"type": "grpc",
	}

	grpcOpts := GetMap(m, "grpc-opts")
	if grpcOpts == nil {
		return transport
	}

	if serviceName := GetStr(grpcOpts, "grpc-service-name"); serviceName != "" {
		transport["service_name"] = serviceName
	}

	return transport
}

// getHeadersMap extracts a headers map from an opts map.
// Handles both map[string]string and map[string]any formats.
func getHeadersMap(opts map[string]any, key string) map[string]any {
	v, ok := opts[key]
	if !ok {
		return nil
	}
	switch h := v.(type) {
	case map[string]any:
		if len(h) == 0 {
			return nil
		}
		return h
	case map[string]string:
		if len(h) == 0 {
			return nil
		}
		result := make(map[string]any, len(h))
		for k, val := range h {
			result[k] = val
		}
		return result
	default:
		return nil
	}
}
