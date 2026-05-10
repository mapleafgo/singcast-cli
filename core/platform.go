package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os/exec"
	"runtime"
	"sync"
	"time"

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

	// Mobile: interface monitor updated via UpdateDefaultInterface callback.
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
}

// UpdateDefaultInterface updates the mobile interface monitor with new data.
func (p *PlatformIO) UpdateDefaultInterface(name string, index int64, expensive bool) {
	p.mu.Lock()
	m := p.ifaceMonitor
	p.mu.Unlock()
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

func (p *PlatformIO) OpenInterface(options *tun.Options, _ option.TunPlatformOptions) (tun.Tun, error) {
	p.mu.Lock()
	fd := p.tunFd
	p.tunFd = 0
	p.mu.Unlock()

	if fd == 0 {
		return nil, fmt.Errorf("no TUN fd available")
	}
	options.FileDescriptor = int(fd)
	return tun.New(*options)
}

func (p *PlatformIO) UsePlatformAutoDetectInterfaceControl() bool { return true }

func (p *PlatformIO) AutoDetectInterfaceControl(fd int) error {
	p.mu.Lock()
	fn := p.protectFn
	p.mu.Unlock()

	if fn != nil {
		if !fn(int32(fd)) {
			return fmt.Errorf("protect fd %d failed", fd)
		}
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
	p.ifaceMonitor = &callbackInterfaceMonitor{}
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
		return nil, fmt.Errorf("no interfaces data")
	}
	return parseAdapterInterfaces(jsonStr)
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
	mu    sync.Mutex
	iface *control.Interface
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
func (m *callbackInterfaceMonitor) RegisterCallback(tun.DefaultInterfaceUpdateCallback) *list.Element[tun.DefaultInterfaceUpdateCallback] {
	return nil
}
func (m *callbackInterfaceMonitor) UnregisterCallback(*list.Element[tun.DefaultInterfaceUpdateCallback]) {}
func (m *callbackInterfaceMonitor) RegisterMyInterface(string) {}
func (m *callbackInterfaceMonitor) MyInterface() string        { return "" }

func (m *callbackInterfaceMonitor) update(name string, index int, expensive bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.iface = &control.Interface{
		Name:  name,
		Index: index,
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
	result := make([]adapter.NetworkInterface, 0, len(mobileIfaces))
	for _, mi := range mobileIfaces {
		addrs, _ := parseInterfaceAddrs(mi.Addresses)
		result = append(result, adapter.NetworkInterface{
			Interface: control.Interface{
				Name:      mi.Name,
				Index:     int(mi.Index),
				MTU:       int(mi.MTU),
				Addresses: addrs,
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

// detectDefaultInterface finds the default network interface by dialing UDP.
// Used on desktop when no platform interface is registered.
func detectDefaultInterface(ctx context.Context) (*control.Interface, error) {
	targets := []string{"8.8.8.8:53", "1.1.1.1:53"}
	for _, target := range targets {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		conn, err := net.DialTimeout("udp4", target, 2*time.Second)
		if err != nil {
			continue
		}
		localAddr := conn.LocalAddr()
		conn.Close()

		localUDP, ok := localAddr.(*net.UDPAddr)
		if !ok {
			continue
		}

		ifaces, err := net.Interfaces()
		if err != nil {
			continue
		}
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if !ok {
					continue
				}
				if ipNet.Contains(localUDP.IP) {
					return &control.Interface{
						Name:  iface.Name,
						Index: iface.Index,
					}, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no default interface found")
}
