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
	"syscall"
	"time"
)

func (s *Server) listenPlatform(ctx context.Context) error {
	os.Remove(s.ipcPath)

	var (
		listener net.Listener
		err      error
	)
	for range 30 {
		listener, err = net.ListenUnix("unix", &net.UnixAddr{
			Name: s.ipcPath,
			Net:  "unix",
		})
		if err == nil {
			break
		}
		// 容器/沙箱可能在启动瞬间以 EROFS 挂载 socket 目录；重试等待挂载完成。
		if !errors.Is(err, syscall.EROFS) {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return fmt.Errorf("listen command sock: %w", err)
	}
	s.listener = listener

	// Set restrictive permissions: only the owner can connect.
	if err := os.Chmod(s.ipcPath, 0o600); err != nil {
		slog.Warn("chmod socket", "error", err)
	}

	// 服务模式：允许 GUI 用户 uid 通过 ACL 连接 socket（免加入组、免重登）
	if guiUID := os.Getenv("SINGCAST_GUI_UID"); guiUID != "" {
		// "0" 是包安装 fallback，对 root 写 ACL 无意义且会误导；跳过。
		if guiUID != "0" {
			aclCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			out, err := exec.CommandContext(aclCtx, "setfacl", "-m", "u:"+guiUID+":rw", s.ipcPath).CombinedOutput()
			cancel()
			if err != nil {
				// 服务模式下 ACL 失败 = GUI 必连不上，显式失败而不是 silent warn
				_ = s.listener.Close()
				os.Remove(s.ipcPath)
				return fmt.Errorf("setfacl for GUI uid %s: %w (%s)", guiUID, err, string(out))
			}
			slog.Info("socket ACL granted", "uid", guiUID)
		}
	}

	dirInfo, err := os.Stat(filepath.Dir(s.ipcPath))
	if err != nil {
		slog.Warn("stat socket dir for chown", "error", err)
	} else if stat, ok := dirInfo.Sys().(*syscall.Stat_t); ok && stat.Uid != 0 {
		if err := os.Chown(s.ipcPath, int(stat.Uid), int(stat.Gid)); err != nil {
			slog.Warn("chown socket", "error", err)
		}
	}

	return nil
}

func (s *Server) cleanupPlatform() {
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.ipcPath)
}
