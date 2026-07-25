//go:build linux

package ipc

import (
	"errors"
	"fmt"
	"os"
)

const polkitRulesPath = "/usr/share/polkit-1/rules.d/singcast.rules"

// InstallService 在 Linux 上写入 polkit 规则，放行 resolved 三条 action。
// 必须以 root 运行；由 GUI 通过 pkexec 或打包 postinst 调用。
func InstallService(_ string) error {
	if os.Geteuid() != 0 {
		return errors.New("service install requires root (run via pkexec or package postinst)")
	}
	if err := os.MkdirAll("/usr/share/polkit-1/rules.d", 0o755); err != nil {
		return fmt.Errorf("create polkit rules dir: %w", err)
	}
	return os.WriteFile(polkitRulesPath, []byte(polkitRules()), 0o644)
}

// UninstallService 删除 polkit 规则文件。
func UninstallService() error {
	if os.Geteuid() != 0 {
		return errors.New("service uninstall requires root")
	}
	err := os.Remove(polkitRulesPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
