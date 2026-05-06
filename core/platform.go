package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/sagernet/sing-box/experimental/libbox"
)

// PlatformIO implements libbox.PlatformInterface with all platform-specific
// state self-contained (no package-level globals).
type PlatformIO struct {
	mu sync.Mutex

	// TUN file descriptor from VpnService (Android) or NetworkExtension (iOS).
	tunFd       int32
	externalTun bool

	// Socket protector from mobile platform (Android VpnService.protect).
	protectFn func(fd int32) bool

	// Interface data from mobile platform (JSON string).
	interfacesJSON string

	// iOS: set to true when the VPN configuration uses includeAllNetworks.
	includeAllNetworks bool

	// Default interface monitor.
	ifaceListener libbox.InterfaceUpdateListener
	pendingUpdate *ifaceUpdate
	cancelDetect  context.CancelFunc

	// Cached TUN options from last OpenTun call.
	tunOptions libbox.TunOptions

	// WiFi state from mobile platform.
	wifiSSID  string
	wifiBSSID string
}

type ifaceUpdate struct {
	name      string
	index     int32
	expensive bool
}

// NewPlatformIO creates a new PlatformIO.
func NewPlatformIO() *PlatformIO {
	return &PlatformIO{}
}

// SetTunFd stores a TUN file descriptor from VpnService or NetworkExtension.
func (p *PlatformIO) SetTunFd(fd int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tunFd = fd
	if fd != 0 {
		p.externalTun = true
	}
	slog.Debug("set TUN fd", "fd", fd, "externalTun", fd != 0)
}

// ResetTunFd clears the stored TUN file descriptor and cached TUN options.
func (p *PlatformIO) ResetTunFd() {
	p.mu.Lock()
	defer p.mu.Unlock()
	oldFd := p.tunFd
	p.tunFd = 0
	p.externalTun = false
	p.tunOptions = nil
	slog.Debug("reset TUN fd", "cleared", oldFd)
}

// SetSocketProtector registers a callback that protects socket fds from VPN routing.
func (p *PlatformIO) SetSocketProtector(fn func(fd int32) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	slog.Debug("[SetSocketProtector]", "registered", fn != nil)
	p.protectFn = fn
}

// SetIncludeAllNetworks sets whether the VPN configuration uses includeAllNetworks (iOS).
func (p *PlatformIO) SetIncludeAllNetworks(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.includeAllNetworks = v
	slog.Debug("set includeAllNetworks", "value", v)
}

// SetWIFIState stores the current WiFi SSID and BSSID from the mobile platform.
func (p *PlatformIO) SetWIFIState(ssid, bssid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.wifiSSID = ssid
	p.wifiBSSID = bssid
}

// SetInterfacesJSON stores interface data from the mobile platform.
func (p *PlatformIO) SetInterfacesJSON(jsonStr string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.interfacesJSON = jsonStr
	slog.Debug("set interfaces JSON", "len", len(jsonStr))
}

// UpdateDefaultInterface reports the current default network interface
// detected by the mobile platform.
func (p *PlatformIO) UpdateDefaultInterface(name string, index int64, expensive bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	slog.Debug("UpdateDefaultInterface", "name", name, "index", index, "expensive", expensive, "hasListener", p.ifaceListener != nil)
	upd := &ifaceUpdate{name: name, index: int32(index), expensive: expensive}
	if p.ifaceListener != nil {
		slog.Debug("UpdateDefaultInterface: calling listener")
		p.ifaceListener.UpdateDefaultInterface(upd.name, upd.index, upd.expensive, false)
	} else {
		slog.Debug("UpdateDefaultInterface: storing pending update")
		p.pendingUpdate = upd
	}
}

// libbox.PlatformInterface implementation.

func (p *PlatformIO) LocalDNSTransport() libbox.LocalDNSTransport { return nil }

func (p *PlatformIO) UsePlatformAutoDetectInterfaceControl() bool {
	slog.Debug("UsePlatformAutoDetectInterfaceControl")
	return true
}

func (p *PlatformIO) AutoDetectInterfaceControl(fd int32) error {
	p.mu.Lock()
	fn := p.protectFn
	p.mu.Unlock()

	if fn != nil {
		ok := fn(fd)
		slog.Debug("AutoDetectInterfaceControl", "fd", fd, "protected", ok)
		if !ok {
			return fmt.Errorf("protect fd %d failed", fd)
		}
	} else {
		slog.Warn("AutoDetectInterfaceControl: no socket protector", "fd", fd)
	}
	return nil
}

func (p *PlatformIO) OpenTun(options libbox.TunOptions) (int32, error) {
	p.mu.Lock()
	fd := p.tunFd
	if fd != 0 {
		p.tunFd = 0
	}
	p.tunOptions = options
	p.mu.Unlock()

	if fd != 0 {
		slog.Debug("[OpenTun] returning fd", "fd", fd)
		return fd, nil
	}
	slog.Warn("[OpenTun] no TUN fd available; mobile platforms must call SetTunFd before starting/reloading")
	return 0, os.ErrInvalid
}

// GetTunOptions returns cached TUN options from the last OpenTun call.
func (p *PlatformIO) GetTunOptions() libbox.TunOptions {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tunOptions
}

// QueryTunOptions returns TUN configuration as JSON for mobile consumers.
func (p *PlatformIO) QueryTunOptions() string {
	opts := p.GetTunOptions()
	if opts == nil {
		return "{}"
	}
	snap := TunOptionsSnapshot{
		MTU:                 opts.GetMTU(),
		AutoRoute:           opts.GetAutoRoute(),
		StrictRoute:         opts.GetStrictRoute(),
		HTTPProxyEnabled:    opts.IsHTTPProxyEnabled(),
		HTTPProxyServer:     opts.GetHTTPProxyServer(),
		HTTPProxyServerPort: opts.GetHTTPProxyServerPort(),
	}
	snap.Inet4Address = routePrefixSlice(opts.GetInet4Address())
	snap.Inet6Address = routePrefixSlice(opts.GetInet6Address())
	if dns, err := opts.GetDNSServerAddress(); err == nil && dns != nil {
		snap.DNSServerAddress = dns.Value
	} else if err != nil {
		slog.Debug("GetDNSServerAddress", "error", err)
	}
	snap.Inet4RouteAddress = routePrefixSlice(opts.GetInet4RouteAddress())
	snap.Inet6RouteAddress = routePrefixSlice(opts.GetInet6RouteAddress())
	snap.Inet4RouteExcludeAddress = routePrefixSlice(opts.GetInet4RouteExcludeAddress())
	snap.Inet6RouteExcludeAddress = routePrefixSlice(opts.GetInet6RouteExcludeAddress())
	snap.Inet4RouteRange = routePrefixSlice(opts.GetInet4RouteRange())
	snap.Inet6RouteRange = routePrefixSlice(opts.GetInet6RouteRange())
	snap.IncludePackage = stringIterSlice(opts.GetIncludePackage())
	snap.ExcludePackage = stringIterSlice(opts.GetExcludePackage())
	snap.HTTPProxyBypassDomain = stringIterSlice(opts.GetHTTPProxyBypassDomain())
	snap.HTTPProxyMatchDomain = stringIterSlice(opts.GetHTTPProxyMatchDomain())
	data, _ := json.Marshal(snap)
	return string(data)
}

// routePrefixSlice converts a RoutePrefixIterator to a string slice.
// Returns nil if the iterator is nil or empty (so omitempty in JSON omits the field).
func routePrefixSlice(iter libbox.RoutePrefixIterator) []string {
	if iter == nil {
		return nil
	}
	var result []string
	for iter.HasNext() {
		p := iter.Next()
		result = append(result, p.String())
	}
	return result
}

// stringIterSlice converts a StringIterator to a string slice.
// Returns nil if the iterator is nil or empty (so omitempty in JSON omits the field).
func stringIterSlice(iter libbox.StringIterator) []string {
	if iter == nil {
		return nil
	}
	var result []string
	for iter.HasNext() {
		result = append(result, iter.Next())
	}
	return result
}

func (p *PlatformIO) FindConnectionOwner(ipProtocol int32, sourceAddress string, sourcePort int32, destinationAddress string, destinationPort int32) (*libbox.ConnectionOwner, error) {
	return findConnectionOwnerImpl(ipProtocol, sourceAddress, sourcePort, destinationAddress, destinationPort)
}

func (p *PlatformIO) UseProcFS() bool { return false }

func (p *PlatformIO) StartDefaultInterfaceMonitor(listener libbox.InterfaceUpdateListener) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ifaceListener = listener
	slog.Debug("[StartDefaultInterfaceMonitor] begin", "hasPendingUpdate", p.pendingUpdate != nil, "os", runtime.GOOS)

	if p.pendingUpdate != nil {
		slog.Debug("[StartDefaultInterfaceMonitor] applying pending update", "name", p.pendingUpdate.name, "index", p.pendingUpdate.index)
		listener.UpdateDefaultInterface(p.pendingUpdate.name, p.pendingUpdate.index, p.pendingUpdate.expensive, false)
		p.pendingUpdate = nil
	} else if runtime.GOOS != "android" && runtime.GOOS != "ios" {
		slog.Debug("[StartDefaultInterfaceMonitor] starting desktop auto-detect goroutine")
		ctx, cancel := context.WithCancel(context.Background())
		p.cancelDetect = cancel
		go detectDefaultInterface(ctx, listener)
	} else {
		slog.Debug("[StartDefaultInterfaceMonitor] mobile: waiting for UpdateDefaultInterface call")
	}
	return nil
}

func (p *PlatformIO) CloseDefaultInterfaceMonitor(listener libbox.InterfaceUpdateListener) error {
	p.mu.Lock()
	if p.cancelDetect != nil {
		p.cancelDetect()
		p.cancelDetect = nil
	}
	p.ifaceListener = nil
	p.mu.Unlock()
	slog.Debug("closing interface monitor")
	return nil
}

func (p *PlatformIO) GetInterfaces() (libbox.NetworkInterfaceIterator, error) {
	p.mu.Lock()
	jsonStr := p.interfacesJSON
	p.mu.Unlock()

	if jsonStr != "" {
		return parseInterfacesJSON(jsonStr)
	}

	// Desktop fallback: use Go's net.Interfaces()
	slog.Warn("GetInterfaces: no cached JSON, using net.Interfaces() fallback")
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	return buildDesktopInterfaces(ifaces)
}

func (p *PlatformIO) UnderNetworkExtension() bool {
	if runtime.GOOS != "ios" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.externalTun
}

func (p *PlatformIO) IncludeAllNetworks() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.includeAllNetworks
}

func (p *PlatformIO) ReadWIFIState() *libbox.WIFIState {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.wifiSSID == "" {
		return nil
	}
	return libbox.NewWIFIState(p.wifiSSID, p.wifiBSSID)
}

func (p *PlatformIO) SystemCertificates() libbox.StringIterator { return nil }

func (p *PlatformIO) ClearDNSCache() { flushSystemDNS() }

// FlushSystemDNS attempts to flush the system DNS cache (best-effort).
func (p *PlatformIO) FlushSystemDNS() { flushSystemDNS() }

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

func (p *PlatformIO) SendNotification(notification *libbox.Notification) error { return nil }

// detectDefaultInterface finds the default network interface by dialing UDP.
func detectDefaultInterface(ctx context.Context, listener libbox.InterfaceUpdateListener) {
	if listener == nil {
		return
	}
	targets := []string{"8.8.8.8:53", "1.1.1.1:53"}
	for attempt := 0; attempt < 5; attempt++ {
		select {
		case <-ctx.Done():
			slog.Debug("detect interface: cancelled")
			return
		default:
		}
		for _, target := range targets {
			select {
			case <-ctx.Done():
				return
			default:
			}
			conn, err := net.DialTimeout("udp4", target, 2*time.Second)
			if err != nil {
				slog.Debug("detect interface: dial failed", "attempt", attempt+1, "target", target, "error", err)
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
						slog.Info("detected default interface", "iface", iface.Name, "index", iface.Index, "via", target)
						listener.UpdateDefaultInterface(iface.Name, int32(iface.Index), false, false)
						return
					}
				}
			}
		}
		slog.Warn("detect interface: attempt failed, retrying", "attempt", attempt+1)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	slog.Warn("detect interface: all attempts failed")
}

type mobileInterface struct {
	Name      string   `json:"name"`
	Index     int32    `json:"index"`
	MTU       int32    `json:"mtu"`
	Addresses []string `json:"addresses"`
	Flags     int32    `json:"flags"`
	Type      int32    `json:"type"`
}

func parseInterfacesJSON(jsonStr string) (libbox.NetworkInterfaceIterator, error) {
	var ifaces []mobileInterface
	if err := json.Unmarshal([]byte(jsonStr), &ifaces); err != nil {
		slog.Error("parse interfaces JSON failed", "error", err)
		return nil, fmt.Errorf("parse interfaces JSON: %w", err)
	}
	var result []*libbox.NetworkInterface
	for _, mi := range ifaces {
		slog.Debug("parsed interface", "name", mi.Name, "index", mi.Index, "flags", mi.Flags, "addrs", len(mi.Addresses))
		result = append(result, &libbox.NetworkInterface{
			Index:     mi.Index,
			MTU:       mi.MTU,
			Name:      mi.Name,
			Addresses: &stringIterator{items: mi.Addresses},
			Flags:     mi.Flags,
			Type:      mi.Type,
		})
	}
	slog.Debug("parsed mobile interfaces", "count", len(result))
	return &networkInterfaceIterator{items: result}, nil
}

func buildDesktopInterfaces(ifaces []net.Interface) (libbox.NetworkInterfaceIterator, error) {
	var result []*libbox.NetworkInterface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		var addrStrings []string
		for _, addr := range addrs {
			addrStrings = append(addrStrings, addr.String())
		}
		ifaceType := int32(libbox.InterfaceTypeOther)
		if iface.Flags&net.FlagBroadcast != 0 {
			ifaceType = libbox.InterfaceTypeEthernet
		}

		var flags int32
		if iface.Flags&net.FlagUp != 0 {
			flags |= 0x1 // IFF_UP
		}
		if iface.Flags&net.FlagRunning != 0 {
			flags |= 0x40 // IFF_RUNNING
		}
		if iface.Flags&net.FlagBroadcast != 0 {
			flags |= 0x2 // IFF_BROADCAST
		}
		if iface.Flags&net.FlagLoopback != 0 {
			flags |= 0x8 // IFF_LOOPBACK
		}
		if iface.Flags&net.FlagPointToPoint != 0 {
			flags |= 0x10 // IFF_POINTOPOINT
		}
		if iface.Flags&net.FlagMulticast != 0 {
			flags |= 0x1000 // IFF_MULTICAST
		}

		result = append(result, &libbox.NetworkInterface{
			Index:     int32(iface.Index),
			MTU:       int32(iface.MTU),
			Name:      iface.Name,
			Addresses: &stringIterator{items: addrStrings},
			Flags:     flags,
			Type:      ifaceType,
		})
	}
	slog.Debug("get interfaces", "count", len(result))
	return &networkInterfaceIterator{items: result}, nil
}

// stringIterator implements libbox.StringIterator.
type stringIterator struct {
	items []string
	index int
}

func (s *stringIterator) Len() int32 { return int32(len(s.items)) }
func (s *stringIterator) Next() string {
	if s.index >= len(s.items) {
		return ""
	}
	item := s.items[s.index]
	s.index++
	return item
}
func (s *stringIterator) HasNext() bool { return s.index < len(s.items) }

// networkInterfaceIterator implements libbox.NetworkInterfaceIterator.
type networkInterfaceIterator struct {
	items []*libbox.NetworkInterface
	index int
}

func (it *networkInterfaceIterator) Next() *libbox.NetworkInterface {
	if it.index >= len(it.items) {
		return nil
	}
	item := it.items[it.index]
	it.index++
	return item
}
func (it *networkInterfaceIterator) HasNext() bool { return it.index < len(it.items) }
