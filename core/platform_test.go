package core

import (
	"encoding/json"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sagernet/sing-box/experimental/libbox"
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

func TestResetTunFd_ClearsState(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(42)
	p.ResetTunFd()
	_, err := p.OpenTun(nil)
	if err == nil {
		t.Fatal("OpenTun should fail after ResetTunFd")
	}
}

// --- OpenTun consumes fd ---

func TestOpenTun_ReturnsExternalFd(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(42)
	fd, err := p.OpenTun(nil)
	if err != nil {
		t.Fatalf("OpenTun: %v", err)
	}
	if fd != 42 {
		t.Fatalf("OpenTun returned fd=%d, want 42", fd)
	}
}

func TestOpenTun_ConsumesFd(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(42)
	_, _ = p.OpenTun(nil)
	_, err := p.OpenTun(nil)
	if err == nil {
		t.Fatal("second OpenTun should fail after fd was consumed")
	}
}

func TestOpenTun_NoFdReturnsError(t *testing.T) {
	p := newPlatform()
	_, err := p.OpenTun(nil)
	if err == nil {
		t.Fatal("OpenTun without prior SetTunFd should return error")
	}
}

// --- GetInterfaces ---

func TestGetInterfaces_OnDesktopReturnsHostInterfaces(t *testing.T) {
	if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		t.Skip("test only runs on desktop platforms")
	}
	p := newPlatform()
	iter, err := p.GetInterfaces()
	if err != nil {
		t.Fatalf("GetInterfaces: %v", err)
	}
	_ = iter
}

func TestGetInterfaces_SetsFlags(t *testing.T) {
	if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		t.Skip("test only runs on desktop platforms")
	}
	p := newPlatform()
	iter, err := p.GetInterfaces()
	if err != nil {
		t.Fatalf("GetInterfaces: %v", err)
	}
	// Verify that returned interfaces have non-zero Flags (IFF_UP must be set).
	for iter.HasNext() {
		iface := iter.Next()
		if iface.Flags == 0 {
			t.Fatalf("interface %q has Flags=0, expected IFF_UP to be set", iface.Name)
		}
		if iface.Flags&1 == 0 { // syscall.IFF_UP = 0x1
			t.Fatalf("interface %q Flags=%d missing IFF_UP bit", iface.Name, iface.Flags)
		}
	}
}

// --- StartDefaultInterfaceMonitor ---

func TestStartDefaultInterfaceMonitor_ReturnsNil(t *testing.T) {
	p := newPlatform()
	err := p.StartDefaultInterfaceMonitor(nil)
	if err != nil {
		t.Fatalf("StartDefaultInterfaceMonitor: %v", err)
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
		t.Fatal("UnderNetworkExtension should return false on non-iOS even with externalTun")
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

// --- parseInterfacesJSON ---

func TestParseInterfacesJSON_Valid(t *testing.T) {
	input := `[{"name":"wlan0","index":3,"mtu":1500,"addresses":["192.168.1.2/24","fe80::1/64"],"flags":1,"type":1}]`
	iter, err := parseInterfacesJSON(input)
	if err != nil {
		t.Fatalf("parseInterfacesJSON: %v", err)
	}
	if !iter.HasNext() {
		t.Fatal("expected at least one interface")
	}
	iface := iter.Next()
	if iface.Name != "wlan0" {
		t.Errorf("Name = %q, want wlan0", iface.Name)
	}
	if iface.Index != 3 {
		t.Errorf("Index = %d, want 3", iface.Index)
	}
	if iface.MTU != 1500 {
		t.Errorf("MTU = %d, want 1500", iface.MTU)
	}
	if iface.Flags != 1 {
		t.Errorf("Flags = %d, want 1", iface.Flags)
	}
	if iface.Type != 1 {
		t.Errorf("Type = %d, want 1", iface.Type)
	}
	// Verify addresses iterator.
	addrs := iface.Addresses
	if addrs.Len() != 2 {
		t.Fatalf("addresses Len = %d, want 2", addrs.Len())
	}
	a1 := addrs.Next()
	if a1 != "192.168.1.2/24" {
		t.Errorf("addr[0] = %q, want 192.168.1.2/24", a1)
	}
	if addrs.Next() != "fe80::1/64" {
		t.Error("addr[1] mismatch")
	}
	if addrs.HasNext() {
		t.Error("should be exhausted")
	}
}

func TestParseInterfacesJSON_Empty(t *testing.T) {
	iter, err := parseInterfacesJSON("[]")
	if err != nil {
		t.Fatalf("parseInterfacesJSON: %v", err)
	}
	if iter.HasNext() {
		t.Error("expected no interfaces for empty array")
	}
}

func TestParseInterfacesJSON_Invalid(t *testing.T) {
	_, err := parseInterfacesJSON("not-json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- SetInterfacesJSON + GetInterfaces interaction ---

func TestGetInterfaces_UsesCachedJSON(t *testing.T) {
	p := newPlatform()
	p.SetInterfacesJSON(`[{"name":"rmnet0","index":5,"mtu":1400,"addresses":["10.0.0.1/32"],"flags":1,"type":0}]`)

	iter, err := p.GetInterfaces()
	if err != nil {
		t.Fatalf("GetInterfaces: %v", err)
	}
	if !iter.HasNext() {
		t.Fatal("expected one interface from cached JSON")
	}
	iface := iter.Next()
	if iface.Name != "rmnet0" {
		t.Errorf("Name = %q, want rmnet0", iface.Name)
	}
}

// --- SetSocketProtector + AutoDetectInterfaceControl ---

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
	// Should not panic, just warn.
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

	// Should not panic, should behave as no protector.
	if err := p.AutoDetectInterfaceControl(42); err != nil {
		t.Fatalf("AutoDetectInterfaceControl: %v", err)
	}
}

// --- IncludeAllNetworks ---

func TestIncludeAllNetworks(t *testing.T) {
	p := newPlatform()
	if p.IncludeAllNetworks() {
		t.Error("default should be false")
	}
	p.SetIncludeAllNetworks(true)
	if !p.IncludeAllNetworks() {
		t.Error("should be true after set")
	}
	p.SetIncludeAllNetworks(false)
	if p.IncludeAllNetworks() {
		t.Error("should be false after clear")
	}
}

// --- UpdateDefaultInterface + pending path ---

type mockIfaceListener struct {
	name      string
	index     int32
	expensive bool
	called    atomic.Int32
}

func (m *mockIfaceListener) UpdateDefaultInterface(name string, index int32, expensive bool, cell bool) {
	m.name = name
	m.index = index
	m.expensive = expensive
	m.called.Add(1)
}

func TestUpdateDefaultInterface_WithListener(t *testing.T) {
	p := newPlatform()
	listener := &mockIfaceListener{}
	p.ifaceListener = listener // set directly for test

	p.UpdateDefaultInterface("wlan0", 3, false)

	if listener.called.Load() != 1 {
		t.Fatal("listener should have been called")
	}
	if listener.name != "wlan0" || listener.index != 3 {
		t.Errorf("listener got name=%q index=%d, want wlan0/3", listener.name, listener.index)
	}
}

func TestUpdateDefaultInterface_PendingWhenNoListener(t *testing.T) {
	p := newPlatform()
	p.UpdateDefaultInterface("rmnet0", 5, true)

	p.mu.Lock()
	upd := p.pendingUpdate
	p.mu.Unlock()
	if upd == nil {
		t.Fatal("expected pending update to be stored")
	}
	if upd.name != "rmnet0" || upd.index != 5 || !upd.expensive {
		t.Errorf("pending = %+v, want rmnet0/5/expensive", upd)
	}
}

func TestStartDefaultInterfaceMonitor_AppliesPendingUpdate(t *testing.T) {
	p := newPlatform()
	p.UpdateDefaultInterface("wlan0", 3, false)

	listener := &mockIfaceListener{}
	if err := p.StartDefaultInterfaceMonitor(listener); err != nil {
		t.Fatalf("StartDefaultInterfaceMonitor: %v", err)
	}

	if listener.called.Load() != 1 {
		t.Fatal("pending update should have been applied to listener")
	}
	if listener.name != "wlan0" {
		t.Errorf("listener name = %q, want wlan0", listener.name)
	}

	// pendingUpdate should be cleared.
	p.mu.Lock()
	if p.pendingUpdate != nil {
		t.Error("pendingUpdate should be nil after being applied")
	}
	p.mu.Unlock()

	// Cleanup.
	if err := p.CloseDefaultInterfaceMonitor(nil); err != nil {
		t.Fatalf("CloseDefaultInterfaceMonitor: %v", err)
	}
}

// --- CloseDefaultInterfaceMonitor ---

func TestCloseDefaultInterfaceMonitor_ClearsListener(t *testing.T) {
	p := newPlatform()
	p.ifaceListener = &mockIfaceListener{}
	if err := p.CloseDefaultInterfaceMonitor(nil); err != nil {
		t.Fatalf("CloseDefaultInterfaceMonitor: %v", err)
	}

	p.mu.Lock()
	if p.ifaceListener != nil {
		t.Error("listener should be nil after close")
	}
	p.mu.Unlock()
}

// --- stringIterator ---

func TestStringIterator(t *testing.T) {
	s := &stringIterator{items: []string{"a", "b", "c"}}
	if s.Len() != 3 {
		t.Errorf("Len = %d, want 3", s.Len())
	}
	if !s.HasNext() {
		t.Error("should have next")
	}
	if s.Next() != "a" || s.Next() != "b" || s.Next() != "c" {
		t.Error("iteration order wrong")
	}
	if s.HasNext() {
		t.Error("should be exhausted")
	}
	if s.Next() != "" {
		t.Error("exhausted should return empty string")
	}
}

func TestStringIterator_Empty(t *testing.T) {
	s := &stringIterator{}
	if s.Len() != 0 {
		t.Errorf("Len = %d, want 0", s.Len())
	}
	if s.HasNext() {
		t.Error("empty should not have next")
	}
}

// --- networkInterfaceIterator ---

func TestNetworkInterfaceIterator(t *testing.T) {
	items := []*libbox.NetworkInterface{
		{Name: "eth0", Index: 1},
		{Name: "wlan0", Index: 2},
	}
	it := &networkInterfaceIterator{items: items}
	if !it.HasNext() {
		t.Error("should have next")
	}
	first := it.Next()
	if first.Name != "eth0" {
		t.Errorf("first = %q, want eth0", first.Name)
	}
	second := it.Next()
	if second.Name != "wlan0" {
		t.Errorf("second = %q, want wlan0", second.Name)
	}
	if it.HasNext() {
		t.Error("should be exhausted")
	}
	if it.Next() != nil {
		t.Error("exhausted should return nil")
	}
}

// --- ClearDNSCache / FlushSystemDNS ---

func TestFlushSystemDNS_NoPanic(t *testing.T) {
	p := newPlatform()
	p.ClearDNSCache()
	p.FlushSystemDNS()
}

// --- ReadWIFIState / SystemCertificates / SendNotification / LocalDNSTransport ---

func TestPlatformIO_NoPanicStubs(t *testing.T) {
	p := newPlatform()
	if p.ReadWIFIState() != nil {
		t.Error("ReadWIFIState should return nil")
	}
	if p.SystemCertificates() != nil {
		t.Error("SystemCertificates should return nil")
	}
	if p.LocalDNSTransport() != nil {
		t.Error("LocalDNSTransport should return nil")
	}
	if err := p.SendNotification(nil); err != nil {
		t.Errorf("SendNotification: %v", err)
	}
}

func TestUsePlatformAutoDetectInterfaceControl(t *testing.T) {
	p := newPlatform()
	if !p.UsePlatformAutoDetectInterfaceControl() {
		t.Error("should return true")
	}
}

func TestUseProcFS(t *testing.T) {
	p := newPlatform()
	if p.UseProcFS() {
		t.Error("UseProcFS should return false (using netlink)")
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

// --- TUN options caching ---

func TestGetTunOptions_NilBeforeOpen(t *testing.T) {
	p := newPlatform()
	if p.GetTunOptions() != nil {
		t.Error("expected nil before OpenTun")
	}
}

func TestQueryTunOptions_EmptyBeforeOpen(t *testing.T) {
	p := newPlatform()
	got := p.QueryTunOptions()
	if got != "{}" {
		t.Errorf("QueryTunOptions before open = %q, want {}", got)
	}
}

func TestRoutePrefixSlice_Nil(t *testing.T) {
	if routePrefixSlice(nil) != nil {
		t.Error("nil iterator should return nil")
	}
}

func TestStringIterSlice_Nil(t *testing.T) {
	if stringIterSlice(nil) != nil {
		t.Error("nil iterator should return nil")
	}
}

func TestResetTunFd_ClearsCachedOptions(t *testing.T) {
	p := newPlatform()
	// Use any non-nil TunOptions to verify ResetTunFd clears it.
	// A simple wrapper suffices since we only test nil/non-nil.
	p.mu.Lock()
	p.tunOptions = noOpTunOptions{}
	p.mu.Unlock()
	if p.GetTunOptions() == nil {
		t.Fatal("expected non-nil after setting tunOptions")
	}
	p.ResetTunFd()
	if p.GetTunOptions() != nil {
		t.Error("ResetTunFd should clear cached tunOptions")
	}
}

type noOpTunOptions struct{}

func (noOpTunOptions) GetInet4Address() libbox.RoutePrefixIterator             { return nil }
func (noOpTunOptions) GetInet6Address() libbox.RoutePrefixIterator             { return nil }
func (noOpTunOptions) GetDNSServerAddress() (*libbox.StringBox, error)         { return nil, nil }
func (noOpTunOptions) GetMTU() int32                                           { return 0 }
func (noOpTunOptions) GetAutoRoute() bool                                      { return false }
func (noOpTunOptions) GetStrictRoute() bool                                    { return false }
func (noOpTunOptions) GetInet4RouteAddress() libbox.RoutePrefixIterator        { return nil }
func (noOpTunOptions) GetInet6RouteAddress() libbox.RoutePrefixIterator        { return nil }
func (noOpTunOptions) GetInet4RouteExcludeAddress() libbox.RoutePrefixIterator { return nil }
func (noOpTunOptions) GetInet6RouteExcludeAddress() libbox.RoutePrefixIterator { return nil }
func (noOpTunOptions) GetInet4RouteRange() libbox.RoutePrefixIterator          { return nil }
func (noOpTunOptions) GetInet6RouteRange() libbox.RoutePrefixIterator          { return nil }
func (noOpTunOptions) GetIncludePackage() libbox.StringIterator                { return nil }
func (noOpTunOptions) GetExcludePackage() libbox.StringIterator                { return nil }
func (noOpTunOptions) IsHTTPProxyEnabled() bool                                { return false }
func (noOpTunOptions) GetHTTPProxyServer() string                              { return "" }
func (noOpTunOptions) GetHTTPProxyServerPort() int32                           { return 0 }
func (noOpTunOptions) GetHTTPProxyBypassDomain() libbox.StringIterator         { return nil }
func (noOpTunOptions) GetHTTPProxyMatchDomain() libbox.StringIterator          { return nil }

// --- WiFi state ---

func TestSetWIFIState_ReadRoundTrip(t *testing.T) {
	p := newPlatform()
	if p.ReadWIFIState() != nil {
		t.Error("should be nil before set")
	}
	p.SetWIFIState("MyWiFi", "aa:bb:cc:dd:ee:ff")
	state := p.ReadWIFIState()
	if state == nil {
		t.Fatal("expected non-nil after set")
	}
}

func TestSetWIFIState_EmptySSIDReturnsNil(t *testing.T) {
	p := newPlatform()
	p.SetWIFIState("", "aa:bb:cc:dd:ee:ff")
	if p.ReadWIFIState() != nil {
		t.Error("empty SSID should return nil")
	}
}
