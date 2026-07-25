//go:build linux

package ipc

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystemdUnit_ContainsKeyFields(t *testing.T) {
	unit := systemdUnit("/opt/Singcast/singcast-core", "/var/lib/singcast", "1000")
	require.Contains(t, unit, "User=singcast")
	require.Contains(t, unit, "Group=singcast")
	require.Contains(t, unit, "AmbientCapabilities=CAP_NET_ADMIN")
	// ExecStart 路径必须带引号，避免空格路径拆词
	require.Contains(t, unit, `ExecStart="/opt/Singcast/singcast-core" ipc --home "/var/lib/singcast"`)
	require.Contains(t, unit, "RuntimeDirectory=singcast")
	require.Contains(t, unit, "StateDirectory=singcast")
	require.Contains(t, unit, "SINGCAST_IPC_PATH=/run/singcast/command.sock")
	require.Contains(t, unit, "SINGCAST_GUI_UID=1000")
}

func TestSystemdUnit_SkipsRootGuiUID(t *testing.T) {
	unit := systemdUnit("/opt/Singcast/singcast-core", "/var/lib/singcast", "0")
	require.NotContains(t, unit, "SINGCAST_GUI_UID=")
}

func TestPolkitRules_ContainsResolvedActions(t *testing.T) {
	rules := polkitRules()
	require.Contains(t, rules, "org.freedesktop.resolve1.set-domains")
	require.Contains(t, rules, "org.freedesktop.resolve1.set-default-route")
	require.Contains(t, rules, "org.freedesktop.resolve1.set-dns-servers")
	require.Contains(t, rules, `subject.user == "singcast"`)
	require.Contains(t, rules, "singcast-core.service")
}

func TestLinuxInstallFiles_AllPaths(t *testing.T) {
	files := linuxInstallFiles("/opt/Singcast/singcast-core", "/var/lib/singcast", "1000")
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	require.Contains(t, paths, "/etc/systemd/system/singcast-core.service")
	require.Contains(t, paths, "/usr/share/polkit-1/rules.d/singcast.rules")
}

func TestLinuxUninstallPaths(t *testing.T) {
	paths := linuxUninstallPaths()
	require.Contains(t, paths, "/etc/systemd/system/singcast-core.service")
	require.Contains(t, paths, "/usr/share/polkit-1/rules.d/singcast.rules")
}

func TestInstallService_RequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("this test verifies non-root rejection")
	}
	err := InstallService("/var/lib/singcast")
	require.Error(t, err)
	require.Contains(t, err.Error(), "root")
}

func TestUninstallService_RequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("this test verifies non-root rejection")
	}
	err := UninstallService()
	require.Error(t, err)
	require.Contains(t, err.Error(), "root")
}

// 防止 lint 报 unused import（strings 在 go vet 下可能被标记）
var _ = strings.Contains
