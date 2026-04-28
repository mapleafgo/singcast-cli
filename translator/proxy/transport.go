package proxy

// TranslateTransport translates mihomo transport/underlay configuration to a sing-box
// transport object. Returns nil if no transport is needed (plain TCP).
//
// Field mappings:
//
//	network: "ws"   -> transport.type: "ws"       (WebSocket)
//	network: "http"  -> transport.type: "http"      (HTTP/2)
//	network: "h2"    -> transport.type: "http"      (H2 uses http transport)
//	network: "grpc"  -> transport.type: "grpc"      (gRPC)
//	network: "tcp"   -> nil (no transport needed)
//
// WebSocket opts: ws-opts.path, ws-opts.headers, ws-opts.max-early-data
// HTTP opts: http-opts.method, http-opts.path, http-opts.headers
// H2 opts: h2-opts.host, h2-opts.path
// gRPC opts: grpc-opts.grpc-service-name
func TranslateTransport(m map[string]any) map[string]any {
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
		return translateWS(m, wsOpts)
	case "http":
		return translateHTTP(m)
	case "h2":
		return translateH2(m)
	case "grpc":
		return translateGRPC(m)
	default:
		return nil
	}
}

// translateWS builds a WebSocket transport from ws-opts.
func translateWS(m map[string]any, wsOpts map[string]any) map[string]any {
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
		transport["max_early_data"] = min(maxEarlyData, 4294967295)
	}

	return transport
}

// translateHTTPUpgrade builds an HTTP Upgrade transport from ws-opts with v2ray-http-upgrade.
func translateHTTPUpgrade(wsOpts map[string]any) map[string]any {
	transport := map[string]any{
		"type": "httpupgrade",
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

	return transport
}

// translateHTTP builds an HTTP transport from http-opts.
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

	// http-opts.path can be a string or []string
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
func translateH2(m map[string]any) map[string]any {
	transport := map[string]any{
		"type": "http",
	}

	h2Opts := GetMap(m, "h2-opts")
	if h2Opts == nil {
		return transport
	}

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
