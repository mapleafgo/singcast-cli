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
	"syscall"
	"time"
)

const (
	// LinuxServiceName 是 systemd unit 名称（不含 .service 后缀）。
	LinuxServiceName = "singcast-core"
	// serviceStateDir 是服务固定 state 目录（StateDirectory=singcast），属主为 serviceUser。
	serviceStateDir = "/var/lib/singcast"
	// serviceBinaryDir 存放 unit ExecStart 指向的二进制，必须 root 独占可写：
	// 服务进程要解析不可信的订阅配置，若能写自己的 ExecStart，一次沦陷即可跨重启持久化。
	serviceBinaryDir = "/usr/local/lib/singcast"
	// serviceUser 是运行服务的系统用户。
	serviceUser = "singcast"
	// cmdTimeout 是 systemctl/useradd 等外部命令的执行上限。
	cmdTimeout = 30 * time.Second
)

// legacyServiceBinaryPath 是历史版本把 core 复制到的位置（在 state 目录内，服务用户可写）。
// 安装时若发现残留会一并删除，避免旧 unit 仍指向可被服务用户改写的二进制。
func legacyServiceBinaryPath() string {
	return filepath.Join(serviceStateDir, "singcast-core")
}

// stableServiceBinaryPath 返回便携/AppImage 场景下复制 core 的固定路径。
// unit 的 ExecStart 必须指向重启后仍存在的路径，不能写 /tmp/.mount_*。
func stableServiceBinaryPath() string {
	return filepath.Join(serviceBinaryDir, "singcast-core")
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
	// 历史位置在服务用户可写目录内，必须迁走（见 serviceBinaryDir 注释）
	if clean == legacyServiceBinaryPath() {
		return true
	}
	// 系统安装前缀：deb/AUR/opt 可被 singcast 用户执行，直接写 unit
	if strings.HasPrefix(clean, "/usr/") ||
		strings.HasPrefix(clean, "/opt/") {
		return false
	}
	// 未知前缀保守复制，避免 unit 指向不可达路径
	return true
}

// installServiceBinary 在需要时把 core 复制到 serviceBinaryDir 下的固定路径。
// 返回应写入 unit 的 ExecStart 路径；不需要复制时原样返回 exe。
// 目标文件保持 root:root 0755——服务用户只需执行权限，给写权限等于允许
// 被攻破的服务改写自己的 ExecStart。必须以 root 调用。
func installServiceBinary(exe string) (string, error) {
	if !needsStableBinaryCopy(exe) {
		return exe, nil
	}
	dst := stableServiceBinaryPath()
	if err := os.MkdirAll(serviceBinaryDir, 0o755); err != nil {
		return "", fmt.Errorf("create service binary dir: %w", err)
	}
	src, err := os.Open(exe)
	if err != nil {
		return "", fmt.Errorf("read service binary: %w", err)
	}
	defer src.Close()

	// 先删再以 O_EXCL|O_NOFOLLOW 创建：O_TRUNC 会跟随符号链接，
	// 使这次 root 写入变成"向链接目标任意路径写内容"的提权原语。
	if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("replace stable service binary: %w", err)
	}
	dstf, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0o755)
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
	slog.Info("installed stable service binary", "path", dst, "from", exe)
	return dst, nil
}

// removeLegacyServiceBinary 删除历史版本留在 state 目录内的 core 副本。
// 该副本属主是服务用户、可被自身改写，留着即是持久化后门。
func removeLegacyServiceBinary() {
	legacy := legacyServiceBinaryPath()
	if err := os.Remove(legacy); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("remove legacy service binary", "path", legacy, "error", err)
		}
		return
	}
	slog.Info("removed legacy service binary", "path", legacy)
}

// InstallService 在 Linux 上安装 systemd 服务、polkit 规则和系统用户。
// 必须以 root 运行；由 GUI 通过 pkexec 或打包 postinst 调用。
// guiUID 从 PKEXEC_UID / SUDO_UID 解析；包安装拿不到时返回 "0"，
// unit 不写 SINGCAST_GUI_UID，留待首次 GUI 开 TUN 时经 pkexec 补 ACL。
// 服务实际 state 固定写死在 unit 的 WorkingDirectory=/var/lib/singcast。
func InstallService(ctx context.Context) error {
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

	if err := ensureServiceUser(ctx); err != nil {
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
	if exe != legacyServiceBinaryPath() {
		removeLegacyServiceBinary()
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

	if err := runCmd(ctx, "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	slog.Info("service install finished")
	return nil
}

// UninstallService 在 Linux 上停止、禁用并删除服务及 polkit 规则。
// 必须以 root 运行。系统用户 singcast 保留（降低卸载副作用）。
func UninstallService(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return errors.New("service uninstall requires root")
	}

	// 停止/禁用，忽略未安装错误
	if err := runCmd(ctx, "systemctl", "stop", LinuxServiceName+".service"); err != nil {
		slog.Warn("stop service during uninstall", "error", err)
	}
	if err := runCmd(ctx, "systemctl", "disable", LinuxServiceName+".service"); err != nil {
		slog.Warn("disable service during uninstall", "error", err)
	}

	for _, p := range linuxUninstallPaths() {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("remove install file", "path", p, "error", err)
		}
	}

	if err := runCmd(ctx, "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	return nil
}

// ensureServiceUser 确保系统用户 singcast 存在。
func ensureServiceUser(ctx context.Context) error {
	if _, err := user.Lookup(serviceUser); err == nil {
		return nil
	}
	return runCmd(ctx, "useradd", "--system", "--no-create-home",
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

// runCmd 执行外部命令并在失败时把 stderr/stdout 一并带进错误。
// 无论调用方 ctx 有无 deadline，都再套 cmdTimeout 上限：install/uninstall 钩子
// 可能被 dpkg/rpm 同步调用，命令挂死会拖住整个包管理事务。
func runCmd(ctx context.Context, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
