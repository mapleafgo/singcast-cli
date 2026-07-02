package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os/exec"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/common/x/list"
)

var _ adapter.PlatformInterface = (*PlatformIO)(nil)

// PlatformIO implements adapter.PlatformInterface for both desktop and mobile.
// Desktop: UsePlatformInterface() = false, TUN handled by sing-box internally.
// Mobile:  UsePlatformInterface() = true, TUN via OpenInterface() with external fd.
type PlatformIO struct {
	isMobile bool

	tunFd           atomic.Int32
	protectFn       atomic.Pointer[func(int32) bool]
	includeAll      atomic.Bool
	getInterfacesFn atomic.Pointer[func() string]
	getWiFiStateFn  atomic.Pointer[func() string]

	// Mobile: interface monitor, created eagerly in SetMobile(true).
	ifaceMonitor *callbackInterfaceMonitor

	// protect counter, incremented on each successful protect call.
	protectCount atomic.Int64
}

func NewPlatformIO() *PlatformIO { return &PlatformIO{} }

// SetMobile marks this instance as mobile. Must be called before StartWithContent.
func (p *PlatformIO) SetMobile(mobile bool) {
	p.isMobile = mobile
	if mobile && p.ifaceMonitor == nil {
		p.ifaceMonitor = &callbackInterfaceMonitor{}
	}
}

func (p *PlatformIO) SetTunFd(fd int32) {
	p.tunFd.Store(fd)
	slog.Debug("set TUN fd", "fd", fd)
}

func (p *PlatformIO) SetSocketProtector(fn func(fd int32) bool) {
	if fn == nil {
		p.protectFn.Store(nil)
	} else {
		p.protectFn.Store(&fn)
	}
	slog.Info("platform: set socket protector", "nil", fn == nil)
}

func (p *PlatformIO) SetIncludeAllNetworks(v bool) {
	p.includeAll.Store(v)
}

func (p *PlatformIO) SetInterfaceProvider(fn func() string) {
	if fn == nil {
		p.getInterfacesFn.Store(nil)
	} else {
		p.getInterfacesFn.Store(&fn)
	}
}

func (p *PlatformIO) SetWiFiStateProvider(fn func() string) {
	if fn == nil {
		p.getWiFiStateFn.Store(nil)
	} else {
		p.getWiFiStateFn.Store(&fn)
	}
}

// UpdateDefaultInterface updates the mobile interface monitor with new data.
func (p *PlatformIO) UpdateDefaultInterface(name string, index int64, expensive bool) {
	m := p.ifaceMonitor
	if m != nil && slices.Contains(m.MyInterfaces(), name) {
		slog.Debug("platform: skip default interface update for TUN", "name", name)
		return
	}
	slog.Info("platform: update default interface", "name", name, "index", index, "monitor", m != nil)
	if m != nil {
		m.update(name, int(index), expensive)
	}
}

// --- adapter.PlatformInterface ---

func (p *PlatformIO) Initialize(mgr adapter.NetworkManager) error {
	if p.ifaceMonitor != nil {
		p.ifaceMonitor.networkMgr = mgr
	}
	return nil
}

func (p *PlatformIO) SetRouter(r adapter.Router) {
	if p.ifaceMonitor != nil {
		p.ifaceMonitor.router = r
	}
}

func (p *PlatformIO) UsePlatformInterface() bool {
	return p.isMobile && p.tunFd.Load() != 0
}

func (p *PlatformIO) IsMobile() bool { return p.isMobile }

func (p *PlatformIO) OpenInterface(options *tun.Options, _ option.TunPlatformOptions) (tun.Tun, error) {
	fd := p.tunFd.Load()
	if fd == 0 {
		return nil, fmt.Errorf("no TUN fd available")
	}
	slog.Debug("platform: opening TUN interface", "fd", fd)
	options.FileDescriptor = int(fd)
	tunDev, err := tun.New(*options)
	if err != nil {
		return nil, err
	}
	p.tunFd.Store(0)
	m := p.ifaceMonitor

	if m != nil {
		if name, nameErr := tunDev.Name(); nameErr == nil {
			m.RegisterMyInterface(name)
			slog.Info("platform: registered TUN interface", "name", name)
		} else {
			slog.Warn("platform: failed to get TUN interface name", "error", nameErr)
		}
	}
	slog.Info("platform: TUN interface opened", "fd", fd)
	return tunDev, nil
}

func (p *PlatformIO) UsePlatformAutoDetectInterfaceControl() bool { return true }

func (p *PlatformIO) AutoDetectInterfaceControl(fd int) error {
	fnPtr := p.protectFn.Load()
	if fnPtr == nil {
		if p.isMobile {
			slog.Warn("platform: protect called but protectFn is nil")
		}
		return nil
	}
	if !(*fnPtr)(int32(fd)) {
		slog.Warn("platform: protect failed", "fd", fd)
		return fmt.Errorf("protect fd %d failed", fd)
	}
	if n := p.protectCount.Add(1); n <= 5 || n%100 == 0 {
		slog.Info("platform: protect ok", "fd", fd, "count", n)
	}
	return nil
}

func (p *PlatformIO) UsePlatformDefaultInterfaceMonitor() bool { return p.isMobile }

func (p *PlatformIO) CreateDefaultInterfaceMonitor(logger.Logger) tun.DefaultInterfaceMonitor {
	if !p.isMobile {
		return nil
	}
	return p.ifaceMonitor
}

func (p *PlatformIO) UsePlatformNetworkInterfaces() bool { return p.isMobile }

func (p *PlatformIO) NetworkInterfaces() ([]adapter.NetworkInterface, error) {
	fn := p.getInterfacesFn.Load()
	if fn == nil {
		return nil, fmt.Errorf("mobile: interface provider not registered")
	}
	return parseAdapterInterfaces((*fn)())
}

func (p *PlatformIO) UnderNetworkExtension() bool {
	if runtime.GOOS != "ios" {
		return false
	}
	return p.tunFd.Load() != 0
}

func (p *PlatformIO) NetworkExtensionIncludeAllNetworks() bool {
	return p.includeAll.Load()
}

func (p *PlatformIO) ClearDNSCache() { flushSystemDNS() }

func (p *PlatformIO) RequestPermissionForWIFIState() error { return nil }

func (p *PlatformIO) ReadWIFIState() adapter.WIFIState {
	fn := p.getWiFiStateFn.Load()
	if fn == nil {
		return adapter.WIFIState{}
	}
	var s struct {
		SSID  string `json:"ssid"`
		BSSID string `json:"bssid"`
	}
	if err := json.Unmarshal([]byte((*fn)()), &s); err != nil {
		slog.Debug("platform: invalid wifi state json", "error", err)
		return adapter.WIFIState{}
	}
	return adapter.WIFIState{SSID: s.SSID, BSSID: s.BSSID}
}

func (p *PlatformIO) SystemCertificates() []string { return nil }

func (p *PlatformIO) UsePlatformConnectionOwnerFinder() bool { return runtime.GOOS == "linux" }

func (p *PlatformIO) FindConnectionOwner(req *adapter.FindConnectionOwnerRequest) (*adapter.ConnectionOwner, error) {
	return findConnectionOwnerImpl(req.IpProtocol, req.SourceAddress, req.SourcePort, req.DestinationAddress, req.DestinationPort)
}

func (p *PlatformIO) UsePlatformWIFIMonitor() bool  { return false }
func (p *PlatformIO) UsePlatformNotification() bool { return false }

func (p *PlatformIO) SendNotification(*adapter.Notification) error { return nil }

func (p *PlatformIO) MyInterfaceAddress() []netip.Addr { return nil }

// --- helpers ---

func flushSystemDNS() {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("resolvectl", "flush-caches")
	case "darwin":
		cmd = exec.Command("dscacheutil", "-flushcache")
	case "windows":
		cmd = exec.Command("ipconfig", "/flushdns")
	default:
		return
	}
	if err := cmd.Run(); err != nil {
		slog.Debug("flush system DNS", "error", err)
	}
}

// callbackInterfaceMonitor implements tun.DefaultInterfaceMonitor for mobile.
// Updated via PlatformIO.UpdateDefaultInterface from the mobile platform.
type callbackInterfaceMonitor struct {
	mu          sync.Mutex
	iface       *control.Interface
	callbacks   list.List[tun.DefaultInterfaceUpdateCallback]
	myInterfaces []string
	networkMgr  adapter.NetworkManager
	router      adapter.Router
}

func (m *callbackInterfaceMonitor) Start() error             { return nil }
func (m *callbackInterfaceMonitor) Close() error             { return nil }
func (m *callbackInterfaceMonitor) OverrideAndroidVPN() bool { return false }
func (m *callbackInterfaceMonitor) AndroidVPNEnabled() bool  { return false }
func (m *callbackInterfaceMonitor) DefaultInterface() *control.Interface {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.iface
}
func (m *callbackInterfaceMonitor) RegisterCallback(cb tun.DefaultInterfaceUpdateCallback) *list.Element[tun.DefaultInterfaceUpdateCallback] {
	m.mu.Lock()
	el := m.callbacks.PushBack(cb)
	iface := m.iface
	m.mu.Unlock()
	slog.Info("platform: register interface callback", "has_iface", iface != nil)
	if iface != nil {
		cb(iface, 0)
	}
	return el
}
func (m *callbackInterfaceMonitor) UnregisterCallback(el *list.Element[tun.DefaultInterfaceUpdateCallback]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks.Remove(el)
}
func (m *callbackInterfaceMonitor) RegisterMyInterface(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.myInterfaces = append(m.myInterfaces, name)
	slog.Info("platform: register my interface", "name", name)
}
func (m *callbackInterfaceMonitor) MyInterfaces() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.myInterfaces
}

func (m *callbackInterfaceMonitor) update(name string, index int, expensive bool) {
	mgr := m.networkMgr

	// Refresh interface list so newly appeared interfaces (e.g. wlan0 after WiFi connect)
	// are visible to InterfaceFinder.ByIndex below.
	if mgr != nil {
		if err := mgr.UpdateInterfaces(); err != nil {
			slog.Warn("platform: refresh interfaces", "error", err)
		}
	}

	m.mu.Lock()

	if index == -1 {
		m.iface = nil
		cbs := m.callbacks.Array()
		m.mu.Unlock()
		slog.Info("platform: interface monitor update", "name", "", "index", -1, "callbacks", len(cbs))
		for _, cb := range cbs {
			cb(nil, 0)
		}
		return
	}

	// Resolve full interface object by index.
	var resolved *control.Interface
	if mgr != nil {
		if found, err := mgr.InterfaceFinder().ByIndex(index); err == nil {
			resolved = found
		} else {
			slog.Warn("platform: find interface by index", "index", index, "error", err)
		}
	}
	if resolved == nil {
		resolved = &control.Interface{Name: name, Index: index}
	}

	// Dedup: skip if interface hasn't changed.
	if m.iface != nil && m.iface.Name == resolved.Name && m.iface.Index == resolved.Index {
		m.mu.Unlock()
		return
	}
	m.iface = resolved
	cbs := m.callbacks.Array()
	m.mu.Unlock()
	slog.Info("platform: interface monitor update", "name", resolved.Name, "index", resolved.Index, "callbacks", len(cbs))
	for _, cb := range cbs {
		cb(resolved, 0)
	}
	if m.router != nil {
		m.router.ResetNetwork()
	}
}

// mobileInterface matches the JSON format from mobile platforms.
type mobileInterface struct {
	Name      string   `json:"name"`
	Index     int32    `json:"index"`
	MTU       int32    `json:"mtu"`
	Addresses []string `json:"addresses"`
	Flags     int32    `json:"flags"`
	Type      int32    `json:"type"`
}

func parseAdapterInterfaces(jsonStr string) ([]adapter.NetworkInterface, error) {
	var mobileIfaces []mobileInterface
	if err := json.Unmarshal([]byte(jsonStr), &mobileIfaces); err != nil {
		return nil, fmt.Errorf("parse interfaces JSON: %w", err)
	}
	slog.Debug("platform: parsing interfaces", "count", len(mobileIfaces))
	result := make([]adapter.NetworkInterface, 0, len(mobileIfaces))
	for _, mi := range mobileIfaces {
		addrs, err := parseInterfaceAddrs(mi.Addresses)
		if err != nil {
			slog.Debug("parse interface addrs", "interface", mi.Name, "error", err)
		}
		result = append(result, adapter.NetworkInterface{
			Interface: control.Interface{
				Name:      mi.Name,
				Index:     int(mi.Index),
				MTU:       int(mi.MTU),
				Addresses: addrs,
				Flags:     net.Flags(mi.Flags),
			},
		})
	}
	return result, nil
}

func parseInterfaceAddrs(addrStrs []string) ([]netip.Prefix, error) {
	var result []netip.Prefix
	for _, s := range addrStrs {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return result, err
		}
		result = append(result, p)
	}
	return result, nil
}
