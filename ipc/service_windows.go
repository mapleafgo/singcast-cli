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

func InstallService(homeDir string) error {
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

	namePtr, err := windows.UTF16PtrFromString(ServiceName)
	if err != nil {
		return err
	}

	// Use windows.OpenService with DELETE only — mgr.OpenService hardcodes
	// SERVICE_ALL_ACCESS which requires standard rights the DACL does not grant.
	h, err := windows.OpenService(m.Handle, namePtr, windows.DELETE)
	if err != nil {
		return fmt.Errorf("open service %s: %w", ServiceName, err)
	}
	defer windows.CloseServiceHandle(h)

	if err := windows.DeleteService(h); err != nil {
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
//	AU (Authenticated Users): 0x100B4 — DELETE + query status + start + stop + interrogate
//	BA (Built-in Admins):     0xF01FF — SERVICE_ALL_ACCESS
//	SY (Local System):        0xF01FF — SERVICE_ALL_ACCESS
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
