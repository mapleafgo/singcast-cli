package proxy

// TranslateUnsupported handles proxy protocols that sing-box does not support.
// It logs a warning and returns nil; translateProxies converts nil results into
// a socks stub outbound so the node stays visible in the Clash API UI.
//
// Unsupported protocols: ssr, snell, ssh, wireguard, mieru, sudoku, masque, trusttunnel.
func TranslateUnsupported(protoName string, m map[string]any, warn func(string)) map[string]any {
	name := GetStr(m, "name")
	if name != "" {
		name = " '" + name + "'"
	}
	warn("protocol '" + protoName + "'" + name + " is not supported by sing-box, degraded to stub")
	return nil
}
