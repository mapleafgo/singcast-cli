//go:build windows

package ipc

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

const ServiceName = "SingcastService"

func InstallService() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	homeDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
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
			DisplayName: "Singcast Service",
			Description: "Singcast proxy core service",
			StartType:   mgr.StartManual,
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

func UninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("service %s not found: %w", ServiceName, err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

// setServiceDACL grants Authenticated Users the rights to start, stop,
// and query the service, so the GUI (running as a normal user) can manage
// the service without needing elevation.
//
// Access mask:
//
//	SERVICE_QUERY_STATUS  = 0x0004
//	SERVICE_START         = 0x0010
//	SERVICE_STOP          = 0x0020
//	SERVICE_INTERROGATE   = 0x0080
//	Total                 = 0x00B4
//
// SIDs: AU = Authenticated Users, BA = Built-in Administrators, SY = Local System.
func setServiceDACL(serviceName string) error {
	sddl := "D:(A;;0x00B4;;;AU)(A;;0x01FF;;;BA)(A;;0x01FF;;;SY)"

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
