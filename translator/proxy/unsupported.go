package proxy

// TranslateUnsupported handles proxy protocols that sing-box does not support.
// It logs a warning and returns nil to indicate the proxy should be skipped.
// See mapping doc section B.14.
//
// Unsupported protocols: ssr, snell, ssh, wireguard, mieru, sudoku, masque, trusttunnel.
func TranslateUnsupported(protoName string, m map[string]any, warn func(string)) map[string]any {
	name := GetStr(m, "name")
	if name != "" {
		name = " '" + name + "'"
	}
	warn("protocol '" + protoName + "'" + name + " is not supported by sing-box, skipping")
	return nil
}
