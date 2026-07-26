package translator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateNoDuplicates(t *testing.T) {
	tt := newTestTranslation()
	tt.config.Outbounds = []map[string]any{
		{"type": "http", "tag": "proxy1"},
		{"type": "socks", "tag": "proxy2"},
	}

	err := validate(tt)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateDuplicateTag(t *testing.T) {
	tt := newTestTranslation()
	tt.config.Outbounds = []map[string]any{
		{"type": "http", "tag": "proxy1"},
		{"type": "socks", "tag": "proxy1"},
	}

	err := validate(tt)

	if err == nil {
		t.Fatal("expected error for duplicate tag, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should contain 'duplicate', got: %v", err)
	}
}

func TestValidateBuiltinOutbounds(t *testing.T) {
	tt := newTestTranslation()
	tt.config.Outbounds = []map[string]any{
		{"type": "http", "tag": "my-proxy"},
	}
	tt.config.Route.Rules = []map[string]any{
		{"outbound": "DIRECT"},
	}

	err := validate(tt)

	if err != nil {
		t.Errorf("expected no error for builtin outbound reference, got %v", err)
	}
}

func TestValidateFinalInvalid(t *testing.T) {
	tt := newTestTranslation()
	tt.config.Route.Final = "nonexistent"

	err := validate(tt)

	if err == nil {
		t.Fatal("expected error for invalid route.final, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should reference 'nonexistent', got: %v", err)
	}
}

func TestValidateFinalValid(t *testing.T) {
	tt := newTestTranslation()
	tt.config.Route.Final = "DIRECT"

	err := validate(tt)

	if err != nil {
		t.Errorf("expected no error for valid route.final 'DIRECT', got %v", err)
	}
}

// 组成员悬空引用必须在翻译阶段拦下：sing-box 对此是启动硬失败。
func TestValidate_RejectsDanglingGroupMember(t *testing.T) {
	tr := newTestTranslation()
	tr.config.Outbounds = []map[string]any{
		{"type": "selector", "tag": "PROXY", "outbounds": []string{"ghost"}},
	}
	err := validate(tr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ghost")
}

// 用户自定义节点与内建 DIRECT 撞名会产出两个同名 outbound。
func TestValidate_RejectsDirectTagCollision(t *testing.T) {
	tr := newTestTranslation()
	tr.config.Outbounds = []map[string]any{
		{"type": "direct", "tag": "DIRECT"},
		{"type": "socks", "tag": "DIRECT"},
	}
	err := validate(tr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "DIRECT")
}

func TestValidate_RejectsDanglingDNSReferences(t *testing.T) {
	t.Run("rule server", func(t *testing.T) {
		tr := newTestTranslation()
		tr.config.DNS = &singboxDNS{
			Servers: []map[string]any{{"tag": "ns-0", "type": "udp"}},
			Rules:   []map[string]any{{"server": "missing"}},
		}
		err := validate(tr)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing")
	})

	t.Run("server detour", func(t *testing.T) {
		tr := newTestTranslation()
		tr.config.DNS = &singboxDNS{
			Servers: []map[string]any{{"tag": "ns-0", "type": "udp", "detour": "no-such-outbound"}},
		}
		err := validate(tr)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no-such-outbound")
	})

	t.Run("valid references pass", func(t *testing.T) {
		tr := newTestTranslation()
		tr.config.DNS = &singboxDNS{
			Servers: []map[string]any{{"tag": "ns-0", "type": "udp", "detour": "DIRECT"}},
			Rules:   []map[string]any{{"server": "ns-0"}},
			Final:   "ns-0",
		}
		require.NoError(t, validate(tr))
	})
}

// 组的丢弃具有传递性：B 被丢弃后，只引用 B 的 A 也必须被丢弃，
// 否则 A 留下悬空引用。单趟清理会漏掉这种情况。
func TestCleanupGroupOutbounds_DropsTransitively(t *testing.T) {
	tr := newTestTranslation()
	// A 只引用 B，B 只引用一个不存在的节点
	groups := []map[string]any{
		{"type": "selector", "tag": "A", "outbounds": []string{"B"}},
		{"type": "selector", "tag": "B", "outbounds": []string{"gone"}},
	}
	translated := map[string]bool{"A": true, "B": true}
	tr.groupTags["A"] = true
	tr.groupTags["B"] = true

	result := cleanupGroupOutbounds(groups, translated, tr)

	require.Empty(t, result, "both groups should be dropped")
	require.False(t, translated["A"], "A must be dropped after B is dropped")
	require.False(t, translated["B"])
}
