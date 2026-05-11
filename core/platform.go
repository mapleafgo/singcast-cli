package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/common/x/list"
)

var _ adapter.PlatformInterface = (*PlatformIO)(nil)

// PlatformIO implements adapter.PlatformInterface for both desktop and mobile.
// Desktop: UsePlatformInterface() = false, TUN handled by sing-box internally.
// Mobile:  UsePlatformInterface() = true, TUN via OpenInterface() with external fd.
type PlatformIO struct {
	mu       sync.Mutex
	isMobile bool

	// Mobile: TUN fd from VpnService (Android) or NetworkExtension (iOS).
	tunFd int32

	// Mobile: socket protector from VpnService.protect.
	protectFn func(fd int32) bool

	// Mobile: interface monitor, created eagerly in SetMobile(true).
	ifaceMonitor *callbackInterfaceMonitor

	// Mobile: interface data as JSON from platform.
	interfacesJSON string

	// iOS: VPN configuration uses includeAllNetworks.
	includeAllNetworks bool

	// WiFi state from mobile platform.
	wifiSSID  string
	wifiBSSID string
}

func NewPlatformIO() *PlatformIO { return &PlatformIO{} }

// SetMobile marks this instance as mobile. Must be called before StartWithContent.
func (p *PlatformIO) SetMobile(mobile bool) {
	p.mu.Lock()
	p.isMobile = mobile
	if mobile && p.ifaceMonitor == nil {
		p.ifaceMonitor = &callbackInterfaceMonitor{}
	}
	p.mu.Unlock()
}

func (p *PlatformIO) SetTunFd(fd int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tunFd = fd
	slog.Debug("set TUN fd", "fd", fd)
}

func (p *PlatformIO) SetSocketProtector(fn func(fd int32) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.protectFn = fn
	slog.Debug("platform: set socket protector", "nil", fn == nil)
}

func (p *PlatformIO) SetIncludeAllNetworks(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.includeAllNetworks = v
}

func (p *PlatformIO) SetWIFIState(ssid, bssid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.wifiSSID = ssid
	p.wifiBSSID = bssid
}

func (p *PlatformIO) SetInterfacesJSON(jsonStr string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.interfacesJSON = jsonStr
	slog.Debug("platform: set interfaces JSON", "len", len(jsonStr))
}

// UpdateDefaultInterface updates the mobile interface monitor with new data.
func (p *PlatformIO) UpdateDefaultInterface(name string, index int64, expensive bool) {
	p.mu.Lock()
	m := p.ifaceMonitor
	p.mu.Unlock()
	slog.Debug("platform: update default interface", "name", name, "index", index, "monitor", m != nil)
	if m != nil {
		m.update(name, int(index), expensive)
	}
}

// --- adapter.PlatformInterface ---

func (p *PlatformIO) Initialize(adapter.NetworkManager) error { return nil }

func (p *PlatformIO) UsePlatformInterface() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isMobile && p.tunFd != 0
}

func (p *PlatformIO) IsMobile() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isMobile
}

func (p *PlatformIO) OpenInterface(options *tun.Options, _ option.TunPlatformOptions) (tun.Tun, error) {
	p.mu.Lock()
	fd := p.tunFd
	p.mu.Unlock()

	if fd == 0 {
		return nil, fmt.Errorf("no TUN fd available")
	}
	slog.Debug("platform: opening TUN interface", "fd", fd)
	options.FileDescriptor = int(fd)
	tunDev, err := tun.New(*options)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.tunFd = 0
	m := p.ifaceMonitor
	p.mu.Unlock()

	if m != nil {
		if name, nameErr := tunDev.Name(); nameErr == nil {
			m.RegisterMyInterface(name)
		}
	}
	slog.Debug("platform: TUN interface opened", "fd", fd)
	return tunDev, nil
}

func (p *PlatformIO) UsePlatformAutoDetectInterfaceControl() bool { return true }

var protectCallCount int64

func (p *PlatformIO) AutoDetectInterfaceControl(fd int) error {
	p.mu.Lock()
	fn := p.protectFn
	p.mu.Unlock()

	if fn == nil {
		return nil
	}
	if !fn(int32(fd)) {
		slog.Warn("platform: protect failed", "fd", fd)
		return fmt.Errorf("protect fd %d failed", fd)
	}
	if n := atomic.AddInt64(&protectCallCount, 1); n <= 5 || n%100 == 0 {
		slog.Debug("platform: protect ok", "fd", fd, "count", n)
	}
	return nil
}

func (p *PlatformIO) UsePlatformDefaultInterfaceMonitor() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isMobile
}

func (p *PlatformIO) CreateDefaultInterfaceMonitor(logger.Logger) tun.DefaultInterfaceMonitor {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.isMobile {
		return nil
	}
	return p.ifaceMonitor
}

func (p *PlatformIO) UsePlatformNetworkInterfaces() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isMobile
}

func (p *PlatformIO) NetworkInterfaces() ([]adapter.NetworkInterface, error) {
	p.mu.Lock()
	jsonStr := p.interfacesJSON
	p.mu.Unlock()

	if jsonStr == "" {
		return nil, fmt.Errorf("mobile: call SetInterfacesJSON() before starting")
	}
	ifaces, err := parseAdapterInterfaces(jsonStr)
	if err != nil {
		return nil, err
	}
	slog.Debug("platform: network interfaces", "count", len(ifaces))
	return ifaces, nil
}

func (p *PlatformIO) UnderNetworkExtension() bool {
	if runtime.GOOS != "ios" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tunFd != 0
}

func (p *PlatformIO) NetworkExtensionIncludeAllNetworks() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.includeAllNetworks
}

func (p *PlatformIO) ClearDNSCache() { flushSystemDNS() }

func (p *PlatformIO) RequestPermissionForWIFIState() error { return nil }

func (p *PlatformIO) ReadWIFIState() adapter.WIFIState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return adapter.WIFIState{SSID: p.wifiSSID, BSSID: p.wifiBSSID}
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
	myInterface string
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
	slog.Debug("platform: register interface callback", "has_iface", iface != nil)
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
	m.myInterface = name
	slog.Debug("platform: register my interface", "name", name)
}
func (m *callbackInterfaceMonitor) MyInterface() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.myInterface
}

func (m *callbackInterfaceMonitor) update(name string, index int, expensive bool) {
	m.mu.Lock()
	iface := &control.Interface{
		Name:  name,
		Index: index,
	}
	m.iface = iface
	n := m.callbacks.Len()
	cbs := m.callbacks
	m.mu.Unlock()
	slog.Debug("platform: interface monitor update", "name", name, "index", index, "callbacks", n)
	for el := cbs.Front(); el != nil; el = el.Next() {
		el.Value(iface, 0)
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
