//go:build !windows

package ipc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// socketUmask 在 bind 期间屏蔽除属主外的全部权限位。
// socket 文件权限决定谁能连上这个特权控制通道，若先按默认 umask 落盘再 chmod，
// 两步之间存在可被本地用户抢连的窗口（socket 目录是 0755 可遍历的）。
const socketUmask = 0o177

func (s *Server) listenPlatform(ctx context.Context) error {
	// 探活后再删：无条件 os.Remove 会让误启动的第二个实例静默抢走路径，
	// 而第一个实例仍在运行且 GUI 再也连不回去。
	if isSocketAlive(ctx, s.ipcPath) {
		return fmt.Errorf("another instance is already listening at %s", s.ipcPath)
	}
	os.Remove(s.ipcPath)

	listener, err := listenUnixPrivate(s.ipcPath)
	if err != nil {
		return fmt.Errorf("listen command sock: %w", err)
	}
	s.listener = listener

	if err := s.grantSocketAccess(ctx); err != nil {
		_ = s.listener.Close()
		os.Remove(s.ipcPath)
		return err
	}
	return nil
}

// isSocketAlive 判断路径上是否已有实例在监听。
// 连得上说明对端活着；ECONNREFUSED 等错误说明是上次异常退出留下的残留文件。
func isSocketAlive(ctx context.Context, path string) bool {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// listenUnixPrivate 在 umask 收紧的前提下 bind unix socket，
// 使 socket 自创建起就只有属主可连，不依赖事后 chmod。
func listenUnixPrivate(path string) (net.Listener, error) {
	old := syscall.Umask(socketUmask)
	defer syscall.Umask(old)

	var (
		listener net.Listener
		err      error
	)
	for range 30 {
		listener, err = net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if err == nil {
			return listener, nil
		}
		// 容器/沙箱可能在启动瞬间以 EROFS 挂载 socket 目录；重试等待挂载完成。
		if !errors.Is(err, syscall.EROFS) {
			return nil, err
		}
		slog.Warn("socket dir read-only, waiting for mount", "path", path)
		time.Sleep(time.Second)
	}
	return nil, err
}

// grantSocketAccess 通过 setfacl 授权 GUI 用户连接 socket：
// 服务模式下是 SINGCAST_GUI_UID 指定的 GUI 用户，否则沿用 socket 目录属主
// （移动端/开发场景下目录已归属最终使用者）。socket 权限是 0600，
// 保留 singcast 属主，用 ACL 添加 GUI 用户的读写权限。
// 因 systemd unit 启用 RestrictSUIDSGID（禁止所有 chown），必须用 setfacl。
func (s *Server) grantSocketAccess(ctx context.Context) error {
	if guiUID := os.Getenv("SINGCAST_GUI_UID"); guiUID != "" && guiUID != "0" {
		// "0" 是包安装拿不到调用者 uid 时的 fallback，此时留待 GUI 首次提权补授权。
		uid, err := strconv.Atoi(guiUID)
		if err != nil {
			return fmt.Errorf("parse SINGCAST_GUI_UID %q: %w", guiUID, err)
		}
		// 用 setfacl 而非 chown：socket 保持 singcast 属主，
		// 服务进程受 RestrictSUIDSGID 约束不能 chown（见 unit 注释）。
		// setfacl 无需额外 capability，acl 已包含在 PKGBUILD depends 中。
		cmd := exec.CommandContext(ctx, "setfacl", "-m", fmt.Sprintf("u:%d:rw", uid), s.ipcPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("setfacl u:%d:rw %s: %w\n%s", uid, s.ipcPath, err, out)
		}
		slog.Info("socket access granted", "uid", uid)
		return nil
	}

	dirInfo, err := os.Stat(filepath.Dir(s.ipcPath))
	if err != nil {
		return fmt.Errorf("stat socket dir: %w", err)
	}
	stat, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid == 0 {
		return nil
	}
	// 非 root 目录（移动端/开发场景）：同样用 setfacl 授权，避免 chown。
	cmd := exec.CommandContext(ctx, "setfacl", "-m", fmt.Sprintf("u:%d:rw", int(stat.Uid)), s.ipcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("setfacl u:%d:rw %s: %w\n%s", int(stat.Uid), s.ipcPath, err, out)
	}
	return nil
}

func (s *Server) cleanupPlatform() {
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.ipcPath)
}
