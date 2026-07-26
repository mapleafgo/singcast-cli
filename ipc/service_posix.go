//go:build darwin

package ipc

import (
	"context"
	"errors"
)

// InstallService 在 macOS 上未实现系统服务安装（Linux 用 systemd，Windows 用 SCM）。
func InstallService(_ context.Context) error {
	return errors.New("service management is not supported on macOS")
}

// UninstallService 在 macOS 上未实现，见 InstallService。
func UninstallService(_ context.Context) error {
	return errors.New("service management is not supported on macOS")
}
