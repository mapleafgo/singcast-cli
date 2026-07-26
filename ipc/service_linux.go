//go:build linux

package ipc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// LinuxServiceName 是 systemd unit 名称（不含 .service 后缀）。
	LinuxServiceName = "singcast-core"
	// serviceStateDir 是服务固定 state 目录（StateDirectory=singcast）。
	serviceStateDir = "/var/lib/singcast"
	// serviceUser 是运行服务的系统用户。
	serviceUser = "singcast"
)

// stableServiceBinaryPath 返回便携/AppImage 场景下复制 core 的固定路径。
// unit 的 ExecStart 必须指向重启后仍存在的路径，不能写 /tmp/.mount_*。
func stableServiceBinaryPath() string {
	return filepath.Join(serviceStateDir, "singcast-core")
}

// needsStableBinaryCopy 判断当前可执行文件是否不宜直接写入 unit ExecStart。
// - AppImage /tmp 挂载点会失效
// - 用户 home 下开发路径：User=singcast 无法穿越 0700 目录（EXEC Permission denied）
func needsStableBinaryCopy(exe string) bool {
	if v := os.Getenv("APPIMAGE"); v != "" {
		return true
	}
	clean := filepath.Clean(exe)
	// 已是稳定安装路径则无需再拷
	if clean == stableServiceBinaryPath() {
		return false
	}
	if strings.HasPrefix(clean, "/tmp/") ||
		strings.HasPrefix(clean, "/home/") ||
		strings.HasPrefix(clean, "/root/") {
		return true
	}
	// 系统安装前缀：deb/AUR/opt 可被 singcast 用户执行，直接写 unit
	if strings.HasPrefix(clean, "/usr/") ||
		strings.HasPrefix(clean, "/opt/") ||
		strings.HasPrefix(clean, "/var/lib/singcast/") {
		return false
	}
	// 未知前缀保守复制，避免 unit 指向不可达路径
	return true
}

// installServiceBinary 在需要时把 core 复制到固定路径，并 chown 给服务用户。
// 返回应写入 unit 的 ExecStart 路径。
func installServiceBinary(exe string) (string, error) {
	if !needsStableBinaryCopy(exe) {
		return exe, nil
	}
	dst := stableServiceBinaryPath()
	if err := os.MkdirAll(serviceStateDir, 0o755); err != nil {
		return "", fmt.Errorf("create state dir for binary: %w", err)
	}
	src, err := os.Open(exe)
	if err != nil {
		return "", fmt.Errorf("read service binary: %w", err)
	}
	defer src.Close()
	dstf, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", fmt.Errorf("create stable service binary: %w", err)
	}
	if _, err := io.Copy(dstf, src); err != nil {
		_ = dstf.Close()
		return "", fmt.Errorf("write stable service binary: %w", err)
	}
	if err := dstf.Close(); err != nil {
		return "", fmt.Errorf("close stable service binary: %w", err)
	}
	if u, err := user.Lookup(serviceUser); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		if err := os.Chown(dst, uid, gid); err != nil {
			slog.Warn("chown stable service binary", "path", dst, "error", err)
		}
	}
	slog.Info("installed stable service binary", "path", dst, "from", exe)
	return dst, nil
}

// InstallService 在 Linux 上安装 systemd 服务、polkit 规则和系统用户。
// 必须以 root 运行；由 GUI 通过 pkexec 或打包 postinst 调用。
// guiUID 从 PKEXEC_UID / SUDO_UID 解析；包安装拿不到时返回 "0"，
// unit 不写 SINGCAST_GUI_UID，留待首次 GUI 开 TUN 时经 pkexec 补 ACL。
// 服务实际 state 固定写死在 unit 的 WorkingDirectory=/var/lib/singcast。
func InstallService() error {
	if os.Geteuid() != 0 {
		return errors.New("service install requires root (run via pkexec or package postinst)")
	}
	slog.Info("service install started")

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
	// StateDirectory 运行时也会建，但 install 阶段先保证属主正确，避免 ipc MkdirAll 0700 失败
	if u, err := user.Lookup(serviceUser); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		if err := os.Chown(serviceStateDir, uid, gid); err != nil {
			slog.Warn("chown state dir", "path", serviceStateDir, "error", err)
		}
	}

	exe, err = installServiceBinary(exe)
	if err != nil {
		return err
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
	slog.Info("service install finished")
	return nil
}

// UninstallService 在 Linux 上停止、禁用并删除服务及 polkit 规则。
// 必须以 root 运行。系统用户 singcast 保留（降低卸载副作用）。
func UninstallService() error {
	if os.Geteuid() != 0 {
		return errors.New("service uninstall requires root")
	}

	// 停止/禁用，忽略未安装错误
	if err := runCmd("systemctl", "stop", LinuxServiceName+".service"); err != nil {
		slog.Warn("stop service during uninstall", "error", err)
	}
	if err := runCmd("systemctl", "disable", LinuxServiceName+".service"); err != nil {
		slog.Warn("disable service during uninstall", "error", err)
	}

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

// effectiveCallerUID 返回发起 install 的真实桌面用户 uid。
// pkexec 下 PKEXEC_UID 有值，sudo 下 SUDO_UID 有值；都拿不到（包安装 root 跑）
// 返回 "0"，使 unit 不写 SINGCAST_GUI_UID——首次 GUI 开 TUN 时经 pkexec 补 ACL。
func effectiveCallerUID() string {
	for _, key := range []string{"PKEXEC_UID", "SUDO_UID"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return "0"
}

func runCmd(name string, args ...string) error {
	// install/uninstall 钩子可能被 dpkg/rpm 同步调用；给外部命令固定超时，避免挂死。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
