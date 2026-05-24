//go:build !windows

package ipc

import "errors"

func InstallService(_ string) error {
	return errors.New("service management is only supported on Windows")
}

func UninstallService() error {
	return errors.New("service management is only supported on Windows")
}
