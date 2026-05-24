package ipc

import "encoding/json"

// JSON-RPC 2.0 method names (GUI → Service).
const (
	MethodStartWithContent     = "core.startWithContent"
	MethodStop                 = "core.stop"
	MethodQueryState           = "core.queryState"
	MethodQueryStats           = "core.queryStats"
	MethodQueryProxies         = "core.queryProxies"
	MethodQueryConnections     = "core.queryConnections"
	MethodQueryMode            = "core.queryMode"
	MethodQueryRules           = "core.queryRules"
	MethodTestDelay            = "core.testDelay"
	MethodTestGroupDelay       = "core.testGroupDelay"
	MethodSelectProxy          = "core.selectProxy"
	MethodSetMode              = "core.setMode"
	MethodCloseConnection      = "core.closeConnection"
	MethodCloseAllConnections  = "core.closeAllConnections"
	MethodSetGroupExpand       = "core.setGroupExpand"
	MethodFlushFakeIP          = "core.flushFakeIP"
	MethodFlushDNSCache        = "core.flushDNSCache"
	MethodFlushSystemDNS       = "core.flushSystemDNS"
	MethodResetNetwork         = "core.resetNetwork"
	MethodTriggerGC            = "core.triggerGC"
	MethodGetVersion           = "core.getVersion"
	MethodCheckConfig          = "core.checkConfig"
	MethodSetLogLevel          = "core.setLogLevel"
	MethodServiceInstall       = "service.install"
	MethodServiceUninstall     = "service.uninstall"
)

// JSON-RPC 2.0 notification names (Service → GUI).
const (
	NotifyLog           = "event.log"
	NotifyURLTest       = "event.urlTest"
	NotifyModeUpdate    = "event.modeUpdate"
	NotifyConnEvent     = "event.connEvent"
	NotifyStateUpdate   = "event.stateUpdate"
	NotifyTrafficUpdate = "event.trafficUpdate"
)

// JSONRPCVersion is the protocol version string.
const JSONRPCVersion = "2.0"

// JSONRPCRequest is a JSON-RPC 2.0 request or notification.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      *int64          `json:"id,omitempty"`
}

// IsNotification returns true if this request has no ID (a notification).
func (r *JSONRPCRequest) IsNotification() bool {
	return r.ID == nil
}

// JSONRPCResponse is a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      *int64          `json:"id"`
}

// JSONRPCError is a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newError(id int64, code int, msg string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Error:   &JSONRPCError{Code: code, Message: msg},
		ID:      ptrInt64(id),
	}
}

func newResult(id int64, v any) JSONRPCResponse {
	data, _ := json.Marshal(v)
	return JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Result:  data,
		ID:      ptrInt64(id),
	}
}

func newEmptyResult(id int64) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Result:  json.RawMessage("null"),
		ID:      ptrInt64(id),
	}
}

func ptrInt64(v int64) *int64 { return &v }

// --- Method parameter types ---

// StartParams holds parameters for core.start.
type StartParams struct {
	Content      string `json:"content"`
	RuleSetProxy string `json:"rule_set_proxy,omitempty"`
}

// TestDelayParams holds parameters for core.testDelay.
type TestDelayParams struct {
	Tag       string `json:"tag"`
	TimeoutMs int32  `json:"timeout_ms"`
}

// TestGroupDelayParams holds parameters for core.testGroupDelay.
type TestGroupDelayParams struct {
	GroupTag  string `json:"group_tag"`
	TimeoutMs int32  `json:"timeout_ms"`
}

// SelectProxyParams holds parameters for core.selectProxy.
type SelectProxyParams struct {
	GroupTag    string `json:"group_tag"`
	OutboundTag string `json:"outbound_tag"`
}

// SetModeParams holds parameters for core.setMode.
type SetModeParams struct {
	Mode string `json:"mode"`
}

// CloseConnectionParams holds parameters for core.closeConnection.
type CloseConnectionParams struct {
	ID string `json:"id"`
}

// SetGroupExpandParams holds parameters for core.setGroupExpand.
type SetGroupExpandParams struct {
	GroupTag string `json:"group_tag"`
	Expand   bool   `json:"expand"`
}

// CheckConfigParams holds parameters for core.checkConfig.
type CheckConfigParams struct {
	Content string `json:"content"`
}

// SetLogLevelParams holds parameters for core.setLogLevel.
type SetLogLevelParams struct {
	Level int32 `json:"level"`
}

// InstallServiceParams holds parameters for service.install.
type InstallServiceParams struct {
	Home string `json:"home"`
}

// --- Notification payload types ---

// StateUpdatePayload represents a kernel state change.
type StateUpdatePayload struct {
	State string `json:"state"`
}

// Notification is a helper to build a JSON-RPC notification message.
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}
