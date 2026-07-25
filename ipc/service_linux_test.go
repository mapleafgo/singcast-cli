//go:build linux

package ipc

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPolkitRules_ContainsResolvedActions(t *testing.T) {
	rules := polkitRules()
	require.Contains(t, rules, "org.freedesktop.resolve1.set-domains")
	require.Contains(t, rules, "org.freedesktop.resolve1.set-default-route")
	require.Contains(t, rules, "org.freedesktop.resolve1.set-dns-servers")
	require.Contains(t, rules, `subject.local == true`)
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
