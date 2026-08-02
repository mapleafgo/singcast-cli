package ipc

import (
	"encoding/json"
	"testing"

	"github.com/mapleafgo/singcast/core"
)

func TestHandleConvert(t *testing.T) {
	h := NewHandler(core.NewService())
	raw := `{"content":"proxies:\n  - name: p\n    type: ss\n    server: 1.2.3.4\n    port: 443\n    cipher: aes-128-gcm\n    password: x\n"}`
	resp := h.Handle(&JSONRPCRequest{
		Method: MethodConvert,
		Params: json.RawMessage(raw),
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["json"] == "" {
		t.Fatal("expected non-empty json result")
	}
}
