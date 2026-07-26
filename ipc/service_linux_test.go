//go:build linux

package ipc

import (
	"context"
	"os"
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
	require.Contains(t, rules, "org.freedesktop.resolve1.revert")
	require.Contains(t, rules, `subject.user == "singcast"`)
	require.Contains(t, rules, "singcast-core.service")
	// 不按 verb 过滤：部分发行版 polkit 细节字段不稳定，unit 匹配即可。
	require.NotContains(t, rules, `action.lookup("verb")`)
	require.Contains(t, rules, "org.freedesktop.systemd1.manage-units")
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
	err := InstallService(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "root")
}

func TestUninstallService_RequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("this test verifies non-root rejection")
	}
	err := UninstallService(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "root")
}

func TestNeedsStableBinaryCopy(t *testing.T) {
	require.True(t, needsStableBinaryCopy("/tmp/.mount_Singca123/singcast-core"))
	require.True(t, needsStableBinaryCopy("/tmp/AppDir/usr/bin/singcast-core"))
	require.True(t, needsStableBinaryCopy("/home/mapleafgo/Projects/OpenProject/singcast/build/linux/x64/release/bundle/singcast-core"))
	require.True(t, needsStableBinaryCopy("/root/singcast-core"))
	t.Setenv("APPIMAGE", "/home/u/Singcast.AppImage")
	require.True(t, needsStableBinaryCopy("/opt/Singcast/singcast-core"))
	// 空值仍算设置了环境变量；显式清除以覆盖非 AppImage 路径
	os.Unsetenv("APPIMAGE")
	require.False(t, needsStableBinaryCopy("/opt/Singcast/singcast-core"))
	require.False(t, needsStableBinaryCopy("/usr/bin/singcast-core"))
	// 已在 root 独占目录内的副本无需再拷
	require.False(t, needsStableBinaryCopy("/usr/local/lib/singcast/singcast-core"))
	// 历史位置（服务用户可写）必须迁走
	require.True(t, needsStableBinaryCopy("/var/lib/singcast/singcast-core"))
}

func TestStableServiceBinaryPath(t *testing.T) {
	require.Equal(t, "/usr/local/lib/singcast/singcast-core", stableServiceBinaryPath())
	// ExecStart 目标不得落在服务用户可写的 state 目录内
	require.NotEqual(t, legacyServiceBinaryPath(), stableServiceBinaryPath())
}

func TestEffectiveCallerUID_FromEnv(t *testing.T) {
	t.Setenv("PKEXEC_UID", "1000")
	t.Setenv("SUDO_UID", "2000")
	require.Equal(t, "1000", effectiveCallerUID())

	t.Setenv("PKEXEC_UID", "")
	require.Equal(t, "2000", effectiveCallerUID())

	t.Setenv("SUDO_UID", "")
	require.Equal(t, "0", effectiveCallerUID())
}

func TestLinuxUninstallPaths_IncludesStableBinary(t *testing.T) {
	paths := linuxUninstallPaths()
	require.Contains(t, paths, stableServiceBinaryPath())
}

func TestSystemdUnit_NoExtraCaps(t *testing.T) {
	unit := systemdUnit("/opt/Singcast/singcast-core", "/var/lib/singcast", "1000")
	require.NotContains(t, unit, "CAP_SYS_PTRACE")
	require.NotContains(t, unit, "CAP_DAC_READ_SEARCH")
	require.Contains(t, unit, "CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE")
}
