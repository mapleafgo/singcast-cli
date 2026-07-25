//go:build linux

package ipc

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// LinuxServiceName 是 systemd unit 名称（不含 .service 后缀）。
	LinuxServiceName = "singcast-core"
	// serviceStateDir 是服务固定 state 目录（StateDirectory=singcast）。
	serviceStateDir = "/var/lib/singcast"
	// serviceUser 是运行服务的系统用户。
	serviceUser = "singcast"
)

// InstallService 在 Linux 上安装 systemd 服务、polkit 规则和系统用户声明。
// 必须以 root 运行；由 GUI 通过 pkexec 或打包 postinst 调用。
func InstallService(_ string) error {
	if os.Geteuid() != 0 {
		return errors.New("service install requires root (run via pkexec or package postinst)")
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	// 解析符号链接（/usr/bin/singcast-core -> /opt/...）
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolve executable symlink: %w", err)
	}

	guiUID := effectiveCallerUID()

	if err := ensureServiceUser(); err != nil {
		return fmt.Errorf("ensure singcast user: %w", err)
	}
	if err := os.MkdirAll(serviceStateDir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	for _, f := range linuxInstallFiles(exe, serviceStateDir, guiUID) {
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", f.Path, err)
		}
		if err := os.WriteFile(f.Path, []byte(f.Content), f.Mode); err != nil {
			return fmt.Errorf("write %s: %w", f.Path, err)
		}
		slog.Info("installed file", "path", f.Path)
	}

	if err := runCmd("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	return nil
}

// UninstallService 在 Linux 上停止、禁用并删除服务及 polkit 规则。
// 必须以 root 运行。系统用户 singcast 保留（降低卸载副作用）。
func UninstallService() error {
	if os.Geteuid() != 0 {
		return errors.New("service uninstall requires root")
	}

	// 停止/禁用，忽略未安装错误
	_ = runCmd("systemctl", "stop", LinuxServiceName+".service")
	_ = runCmd("systemctl", "disable", LinuxServiceName+".service")

	for _, p := range linuxUninstallPaths() {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("remove install file", "path", p, "error", err)
		}
	}

	if err := runCmd("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	return nil
}

// ensureServiceUser 确保系统用户 singcast 存在。
func ensureServiceUser() error {
	if _, err := user.Lookup(serviceUser); err == nil {
		return nil
	}
	return runCmd("useradd", "--system", "--no-create-home",
		"--home-dir", serviceStateDir,
		"--shell", "/usr/sbin/nologin",
		serviceUser)
}

// effectiveCallerUID 返回发起 install 的真实用户 uid。
// pkexec 下 PKEXEC_UID / SUDO_UID 有值；打包 postinst 下 fallback 到当前 uid。
func effectiveCallerUID() string {
	for _, key := range []string{"PKEXEC_UID", "SUDO_UID"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	if name := os.Getenv("SUDO_USER"); name != "" {
		if u, err := user.Lookup(name); err == nil {
			return u.Uid
		}
	}
	return strconv.Itoa(os.Geteuid())
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
