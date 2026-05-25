package ipc

import (
	"encoding/json"
	"fmt"

	"github.com/mapleafgo/singcast/core"
)

// Handler routes JSON-RPC requests to core.Service methods.
type Handler struct {
	svc *core.Service
}

// NewHandler creates a new Handler wrapping the given core.Service.
func NewHandler(svc *core.Service) *Handler {
	return &Handler{svc: svc}
}

// Handle processes a JSON-RPC request and returns a response.
func (h *Handler) Handle(req *JSONRPCRequest) JSONRPCResponse {
	id := int64(0)
	if req.ID != nil {
		id = *req.ID
	}

	switch req.Method {
	case MethodStartWithContent:
		return h.handleStartWithContent(req, id)
	case MethodStop:
		return h.handleVoid(req, id, func() error { return h.svc.Stop() })
	case MethodQueryState:
		return newResult(id, h.svc.State().String())
	case MethodQueryStats:
		return h.handleRawJSON(id, h.svc.QueryStats())
	case MethodQueryProxies:
		return h.handleRawJSON(id, h.svc.QueryProxies())
	case MethodQueryConnections:
		return h.handleRawJSON(id, h.svc.QueryConnections())
	case MethodQueryMode:
		return h.handleRawJSON(id, h.svc.QueryMode())
	case MethodQueryRules:
		return h.handleRawJSON(id, h.svc.QueryRules())
	case MethodTestDelay:
		return h.handleTestDelay(req, id)
	case MethodTestGroupDelay:
		return h.handleTestGroupDelay(req, id)
	case MethodSelectProxy:
		return h.handleSelectProxy(req, id)
	case MethodSetMode:
		return h.handleSetMode(req, id)
	case MethodCloseConnection:
		return h.handleCloseConnection(req, id)
	case MethodCloseAllConnections:
		return h.handleVoid(req, id, func() error { return h.svc.CloseConnections() })
	case MethodSetGroupExpand:
		return h.handleSetGroupExpand(req, id)
	case MethodFlushFakeIP:
		return h.handleVoid(req, id, func() error { return h.svc.FlushFakeIP() })
	case MethodFlushDNSCache:
		return h.handleVoid(req, id, func() error { return h.svc.FlushDNSCache() })
	case MethodFlushSystemDNS:
		h.svc.FlushSystemDNS()
		return newEmptyResult(id)
	case MethodResetNetwork:
		h.svc.ResetNetwork()
		return newEmptyResult(id)
	case MethodTriggerGC:
		h.svc.TriggerGC()
		return newEmptyResult(id)
	case MethodGetVersion:
		return h.handleRawJSON(id, core.VersionJSON())
	case MethodCheckConfig:
		return h.handleCheckConfig(req, id)
	case MethodSetLogLevel:
		return h.handleSetLogLevel(req, id)
	default:
		return newError(id, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (h *Handler) handleStartWithContent(req *JSONRPCRequest, id int64) JSONRPCResponse {
	var params StartParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newError(id, -32602, "invalid params: "+err.Error())
	}
	if err := h.svc.StartWithContent(params.Content, params.RuleSetProxy); err != nil {
		return newError(id, 1, err.Error())
	}
	return newEmptyResult(id)
}

func (h *Handler) handleTestDelay(req *JSONRPCRequest, id int64) JSONRPCResponse {
	var params TestDelayParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newError(id, -32602, "invalid params: "+err.Error())
	}
	delay := h.svc.URLTest(params.Tag, params.TimeoutMs)
	return newResult(id, map[string]int32{"delay": delay})
}

func (h *Handler) handleTestGroupDelay(req *JSONRPCRequest, id int64) JSONRPCResponse {
	var params TestGroupDelayParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newError(id, -32602, "invalid params: "+err.Error())
	}
	result := h.svc.TestGroupDelay(params.GroupTag, params.TimeoutMs)
	return h.handleRawJSON(id, result)
}

func (h *Handler) handleSelectProxy(req *JSONRPCRequest, id int64) JSONRPCResponse {
	var params SelectProxyParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newError(id, -32602, "invalid params: "+err.Error())
	}
	if err := h.svc.SelectOutbound(params.GroupTag, params.OutboundTag); err != nil {
		return newError(id, 1, err.Error())
	}
	return newEmptyResult(id)
}

func (h *Handler) handleSetMode(req *JSONRPCRequest, id int64) JSONRPCResponse {
	var params SetModeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newError(id, -32602, "invalid params: "+err.Error())
	}
	if err := h.svc.SetMode(params.Mode); err != nil {
		return newError(id, 1, err.Error())
	}
	return newEmptyResult(id)
}

func (h *Handler) handleCloseConnection(req *JSONRPCRequest, id int64) JSONRPCResponse {
	var params CloseConnectionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newError(id, -32602, "invalid params: "+err.Error())
	}
	if err := h.svc.CloseConnection(params.ID); err != nil {
		return newError(id, 1, err.Error())
	}
	return newEmptyResult(id)
}

func (h *Handler) handleSetGroupExpand(req *JSONRPCRequest, id int64) JSONRPCResponse {
	var params SetGroupExpandParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newError(id, -32602, "invalid params: "+err.Error())
	}
	if err := h.svc.SetGroupExpand(params.GroupTag, params.Expand); err != nil {
		return newError(id, 1, err.Error())
	}
	return newEmptyResult(id)
}

func (h *Handler) handleCheckConfig(req *JSONRPCRequest, id int64) JSONRPCResponse {
	var params CheckConfigParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newError(id, -32602, "invalid params: "+err.Error())
	}
	if err := core.CheckConfig(params.Content); err != nil {
		return newResult(id, map[string]string{"error": err.Error()})
	}
	return newResult(id, map[string]string{"error": ""})
}

func (h *Handler) handleSetLogLevel(req *JSONRPCRequest, id int64) JSONRPCResponse {
	var params SetLogLevelParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newError(id, -32602, "invalid params: "+err.Error())
	}
	h.svc.SetLogLevel(params.Level)
	return newEmptyResult(id)
}

func (h *Handler) handleVoid(req *JSONRPCRequest, id int64, fn func() error) JSONRPCResponse {
	if err := fn(); err != nil {
		return newError(id, 1, err.Error())
	}
	return newEmptyResult(id)
}

func (h *Handler) handleRawJSON(id int64, raw string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Result:  json.RawMessage(raw),
		ID:      ptrInt64(id),
	}
}
