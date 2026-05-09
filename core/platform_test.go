package core

import (
	"encoding/json"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sagernet/sing-box/option"
)

func newPlatform() *PlatformIO {
	return NewPlatformIO()
}

// --- SetTunFd / ResetTunFd ---

func TestSetTunFd_ZeroDoesNotPanic(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(0)
}

func TestResetTunFd_WithoutSetDoesNotPanic(t *testing.T) {
	p := newPlatform()
	p.ResetTunFd()
}

// --- UsePlatformInterface ---

func TestUsePlatformInterface_DefaultFalse(t *testing.T) {
	p := newPlatform()
	if p.UsePlatformInterface() {
		t.Error("default should be desktop (false)")
	}
}

func TestUsePlatformInterface_MobileTrue(t *testing.T) {
	p := newPlatform()
	p.SetMobile(true)
	if !p.UsePlatformInterface() {
		t.Error("should be true after SetMobile(true)")
	}
	p.SetMobile(false)
	if p.UsePlatformInterface() {
		t.Error("should be false after SetMobile(false)")
	}
}

// --- UsePlatformDefaultInterfaceMonitor ---

func TestUsePlatformDefaultInterfaceMonitor_Desktop(t *testing.T) {
	p := newPlatform()
	if p.UsePlatformDefaultInterfaceMonitor() {
		t.Error("desktop should not use platform interface monitor")
	}
}

func TestUsePlatformDefaultInterfaceMonitor_Mobile(t *testing.T) {
	p := newPlatform()
	p.SetMobile(true)
	if !p.UsePlatformDefaultInterfaceMonitor() {
		t.Error("mobile should use platform interface monitor")
	}
}

// --- CreateDefaultInterfaceMonitor ---

func TestCreateDefaultInterfaceMonitor_DesktopNil(t *testing.T) {
	p := newPlatform()
	if p.CreateDefaultInterfaceMonitor(nil) != nil {
		t.Error("desktop should return nil monitor")
	}
}

func TestCreateDefaultInterfaceMonitor_MobileNonNil(t *testing.T) {
	p := newPlatform()
	p.SetMobile(true)
	m := p.CreateDefaultInterfaceMonitor(nil)
	if m == nil {
		t.Fatal("mobile should return non-nil monitor")
	}
}

// --- UsePlatformNetworkInterfaces ---

func TestUsePlatformNetworkInterfaces_Desktop(t *testing.T) {
	p := newPlatform()
	if p.UsePlatformNetworkInterfaces() {
		t.Error("desktop should not use platform network interfaces")
	}
}

func TestUsePlatformNetworkInterfaces_Mobile(t *testing.T) {
	p := newPlatform()
	p.SetMobile(true)
	if !p.UsePlatformNetworkInterfaces() {
		t.Error("mobile should use platform network interfaces")
	}
}

// --- NetworkInterfaces ---

func TestNetworkInterfaces_NoData(t *testing.T) {
	p := newPlatform()
	_, err := p.NetworkInterfaces()
	if err == nil {
		t.Error("expected error when no interfaces data")
	}
}

func TestNetworkInterfaces_ValidJSON(t *testing.T) {
	p := newPlatform()
	p.SetInterfacesJSON(`[{"name":"wlan0","index":3,"mtu":1500,"addresses":["192.168.1.2/24"],"flags":1,"type":1}]`)

	ifaces, err := p.NetworkInterfaces()
	if err != nil {
		t.Fatalf("NetworkInterfaces: %v", err)
	}
	if len(ifaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(ifaces))
	}
	if ifaces[0].Name != "wlan0" {
		t.Errorf("Name = %q, want wlan0", ifaces[0].Name)
	}
	if ifaces[0].Index != 3 {
		t.Errorf("Index = %d, want 3", ifaces[0].Index)
	}
}

// --- AutoDetectInterfaceControl ---

func TestAutoDetectInterfaceControl_WithProtector(t *testing.T) {
	p := newPlatform()
	var called atomic.Int32
	p.SetSocketProtector(func(fd int32) bool {
		called.Store(fd)
		return true
	})

	if err := p.AutoDetectInterfaceControl(99); err != nil {
		t.Fatalf("AutoDetectInterfaceControl: %v", err)
	}
	if called.Load() != 99 {
		t.Errorf("protector called with fd=%d, want 99", called.Load())
	}
}

func TestAutoDetectInterfaceControl_NoProtector(t *testing.T) {
	p := newPlatform()
	if err := p.AutoDetectInterfaceControl(42); err != nil {
		t.Fatalf("AutoDetectInterfaceControl without protector: %v", err)
	}
}

func TestAutoDetectInterfaceControl_ProtectorFails(t *testing.T) {
	p := newPlatform()
	p.SetSocketProtector(func(fd int32) bool { return false })

	err := p.AutoDetectInterfaceControl(42)
	if err == nil {
		t.Fatal("expected error when protector returns false")
	}
}

func TestAutoDetectInterfaceControl_NilProtectorClears(t *testing.T) {
	p := newPlatform()
	p.SetSocketProtector(func(fd int32) bool { return true })
	p.SetSocketProtector(nil)

	if err := p.AutoDetectInterfaceControl(42); err != nil {
		t.Fatalf("AutoDetectInterfaceControl: %v", err)
	}
}

// --- UnderNetworkExtension ---

func TestUnderNetworkExtension_NonIosReturnsFalse(t *testing.T) {
	if runtime.GOOS == "ios" {
		t.Skip("test only runs on non-iOS")
	}
	p := newPlatform()
	p.SetTunFd(42)
	if p.UnderNetworkExtension() {
		t.Fatal("UnderNetworkExtension should return false on non-iOS")
	}
}

// --- NetworkExtensionIncludeAllNetworks ---

func TestNetworkExtensionIncludeAllNetworks(t *testing.T) {
	p := newPlatform()
	if p.NetworkExtensionIncludeAllNetworks() {
		t.Error("default should be false")
	}
	p.SetIncludeAllNetworks(true)
	if !p.NetworkExtensionIncludeAllNetworks() {
		t.Error("should be true after set")
	}
}

// --- WiFi state ---

func TestSetWIFIState_ReadRoundTrip(t *testing.T) {
	p := newPlatform()
	state := p.ReadWIFIState()
	if state.SSID != "" {
		t.Error("should be empty before set")
	}
	p.SetWIFIState("MyWiFi", "aa:bb:cc:dd:ee:ff")
	state = p.ReadWIFIState()
	if state.SSID != "MyWiFi" || state.BSSID != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("got SSID=%q BSSID=%q", state.SSID, state.BSSID)
	}
}

// --- UpdateDefaultInterface ---

func TestUpdateDefaultInterface_UpdatesMonitor(t *testing.T) {
	p := newPlatform()
	p.SetMobile(true)
	m := p.CreateDefaultInterfaceMonitor(nil)
	if m == nil {
		t.Fatal("expected non-nil monitor")
	}

	p.UpdateDefaultInterface("wlan0", 3, false)

	iface := m.DefaultInterface()
	if iface == nil {
		t.Fatal("expected non-nil interface after update")
	}
	if iface.Name != "wlan0" || iface.Index != 3 {
		t.Errorf("got name=%q index=%d, want wlan0/3", iface.Name, iface.Index)
	}
}

// --- concurrency safety ---

func TestTunState_ConcurrentAccess(t *testing.T) {
	p := newPlatform()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); p.SetTunFd(int32(i%2) * 10) }()
		go func() { defer wg.Done(); p.UnderNetworkExtension() }()
		go func() { defer wg.Done(); p.ResetTunFd() }()
	}
	wg.Wait()
}

// --- parseAdapterInterfaces ---

func TestParseAdapterInterfaces_Valid(t *testing.T) {
	input := `[{"name":"wlan0","index":3,"mtu":1500,"addresses":["192.168.1.2/24","fe80::1/64"],"flags":1,"type":1}]`
	ifaces, err := parseAdapterInterfaces(input)
	if err != nil {
		t.Fatalf("parseAdapterInterfaces: %v", err)
	}
	if len(ifaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(ifaces))
	}
	if ifaces[0].Name != "wlan0" {
		t.Errorf("Name = %q, want wlan0", ifaces[0].Name)
	}
	if ifaces[0].Index != 3 {
		t.Errorf("Index = %d, want 3", ifaces[0].Index)
	}
	if ifaces[0].MTU != 1500 {
		t.Errorf("MTU = %d, want 1500", ifaces[0].MTU)
	}
}

func TestParseAdapterInterfaces_Empty(t *testing.T) {
	ifaces, err := parseAdapterInterfaces("[]")
	if err != nil {
		t.Fatalf("parseAdapterInterfaces: %v", err)
	}
	if len(ifaces) != 0 {
		t.Error("expected no interfaces for empty array")
	}
}

func TestParseAdapterInterfaces_Invalid(t *testing.T) {
	_, err := parseAdapterInterfaces("not-json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- mobileInterface JSON round-trip ---

func TestMobileInterfaceJSON(t *testing.T) {
	mi := []mobileInterface{
		{Name: "wlan0", Index: 3, MTU: 1500, Addresses: []string{"192.168.1.2/24"}, Flags: 0x1043, Type: 1},
	}
	data, err := json.Marshal(mi)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed []mobileInterface
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed) != 1 || parsed[0].Name != "wlan0" {
		t.Errorf("round-trip failed: %v", parsed)
	}
	if parsed[0].Flags != 0x1043 {
		t.Errorf("Flags = %d, want 0x1043", parsed[0].Flags)
	}
}

// --- FlushSystemDNS / ClearDNSCache ---

func TestFlushSystemDNS_NoPanic(t *testing.T) {
	p := newPlatform()
	p.ClearDNSCache()
	flushSystemDNS()
}

// --- Stub methods ---

func TestPlatformIO_NoPanicStubs(t *testing.T) {
	p := newPlatform()
	if p.ReadWIFIState().SSID != "" {
		t.Error("ReadWIFIState should return empty SSID by default")
	}
	if p.SystemCertificates() != nil {
		t.Error("SystemCertificates should return nil")
	}
	if err := p.SendNotification(nil); err != nil {
		t.Errorf("SendNotification: %v", err)
	}
	if p.MyInterfaceAddress() != nil {
		t.Error("MyInterfaceAddress should return nil")
	}
}

func TestUsePlatformAutoDetectInterfaceControl(t *testing.T) {
	p := newPlatform()
	if !p.UsePlatformAutoDetectInterfaceControl() {
		t.Error("should return true")
	}
}

func TestUsePlatformConnectionOwnerFinder(t *testing.T) {
	p := newPlatform()
	want := runtime.GOOS == "linux"
	if p.UsePlatformConnectionOwnerFinder() != want {
		t.Errorf("UsePlatformConnectionOwnerFinder = %v, want %v", !want, want)
	}
}

func TestPlatformIO_Initialize(t *testing.T) {
	p := newPlatform()
	if err := p.Initialize(nil); err != nil {
		t.Errorf("Initialize: %v", err)
	}
}

// skip on non-Linux (no root needed for unit tests)
func TestFindConnectionOwner_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("tested separately on Linux")
	}
	p := newPlatform()
	_, err := p.FindConnectionOwner(nil)
	if err == nil {
		t.Error("expected error on non-Linux")
	}
}

// --- callbackInterfaceMonitor ---

func TestCallbackInterfaceMonitor_DefaultNil(t *testing.T) {
	m := &callbackInterfaceMonitor{}
	if m.DefaultInterface() != nil {
		t.Error("default should be nil")
	}
}

func TestCallbackInterfaceMonitor_Lifecycle(t *testing.T) {
	m := &callbackInterfaceMonitor{}
	if err := m.Start(); err != nil {
		t.Errorf("Start: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// --- SetInterfacesJSON + NetworkInterfaces interaction ---

func TestNetworkInterfaces_UsesCachedJSON(t *testing.T) {
	p := newPlatform()
	p.SetInterfacesJSON(`[{"name":"rmnet0","index":5,"mtu":1400,"addresses":["10.0.0.1/32"],"flags":1,"type":0}]`)

	ifaces, err := p.NetworkInterfaces()
	if err != nil {
		t.Fatalf("NetworkInterfaces: %v", err)
	}
	if len(ifaces) != 1 {
		t.Fatal("expected one interface from cached JSON")
	}
	if ifaces[0].Name != "rmnet0" {
		t.Errorf("Name = %q, want rmnet0", ifaces[0].Name)
	}
}

func TestOpenInterface_NoFdError(t *testing.T) {
	p := newPlatform()
	p.SetMobile(true)
	_, err := p.OpenInterface(nil, option.TunPlatformOptions{})
	if err == nil {
		t.Error("OpenInterface should fail without TUN fd")
	}
}
