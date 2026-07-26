//go:build windows

package ipc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

const ServiceName = "SingcastService"

func InstallService(_ context.Context) error {
	// Windows 推导用户数据目录（与 Flutter getApplicationSupportDirectory 一致）
	configDir, err := os.UserConfigDir() // %APPDATA% on Windows
	if err != nil {
		return fmt.Errorf("resolve user config dir: %w", err)
	}
	homeDir := filepath.Join(configDir, "cn.mapleafgo.singcast")
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err == nil {
		// Service already exists — update config and DACL to latest.
		defer s.Close()
		if err := s.UpdateConfig(mgr.Config{
			DisplayName:    "Singcast Service",
			Description:    "Singcast proxy core service",
			StartType:      mgr.StartManual,
			BinaryPathName: fmt.Sprintf(`"%s" ipc --home "%s"`, exe, homeDir),
		}); err != nil {
			return fmt.Errorf("update service config: %w", err)
		}
		return setServiceDACL(ServiceName)
	}

	s, err = m.CreateService(ServiceName, exe, mgr.Config{
		DisplayName: "Singcast Service",
		Description: "Singcast proxy core service",
		StartType:   mgr.StartManual,
	}, "ipc", "--home", homeDir)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	s.Close()

	return setServiceDACL(ServiceName)
}

func UninstallService(_ context.Context) error {
	// Connect to SCM with minimal access — SC_MANAGER_CONNECT is granted to
	// Authenticated Users by default, so no elevation is needed.
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer windows.CloseServiceHandle(scm)

	namePtr, err := windows.UTF16PtrFromString(ServiceName)
	if err != nil {
		return err
	}

	// Open the service with the minimum rights needed: STOP + QUERY_STATUS + DELETE.
	// The DACL set during install grants AU these rights, so no elevation is needed.
	h, err := windows.OpenService(scm, namePtr,
		windows.SERVICE_STOP|windows.SERVICE_QUERY_STATUS|windows.DELETE)
	if err != nil {
		return fmt.Errorf("open service %s: %w", ServiceName, err)
	}
	defer windows.CloseServiceHandle(h)

	// Stop the service first. ERROR_SERVICE_NOT_ACTIVE is harmless.
	var svcStatus windows.SERVICE_STATUS
	if err := windows.ControlService(h, windows.SERVICE_CONTROL_STOP, &svcStatus); err != nil {
		if !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
			return fmt.Errorf("stop service: %w", err)
		}
	} else {
		waitForStopped(h) // best-effort, proceed to delete regardless
	}

	if err := windows.DeleteService(h); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

// waitForStopped waits for the service to reach STOPPED.
// Returns true if stopped, false if timed out — the caller should
// proceed with DeleteService either way; SCM handles lazy cleanup.
func waitForStopped(h windows.Handle) bool {
	const timeout = 5 * time.Second
	const interval = 200 * time.Millisecond
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var status windows.SERVICE_STATUS
		if windows.QueryServiceStatus(h, &status) != nil {
			return false
		}
		if status.CurrentState == windows.SERVICE_STOPPED {
			return true
		}
		time.Sleep(interval)
	}
	return false
}

// setServiceDACL grants Authenticated Users the rights to start, stop,
// and query the service, so the GUI (running as a normal user) can manage
// the service without needing elevation.
func setServiceDACL(serviceName string) error {
	sddl := "D:(A;;0x100B4;;;AU)(A;;0xF01FF;;;BA)(A;;0xF01FF;;;SY)"

	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("parse SDDL: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(sd)))

	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("get DACL: %w", err)
	}

	return windows.SetNamedSecurityInfo(
		serviceName,
		windows.SE_SERVICE,
		windows.DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
}
