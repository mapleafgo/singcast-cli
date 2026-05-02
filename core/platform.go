package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/sagernet/sing-box/experimental/libbox"
)

// SocketProtector protects socket file descriptors from being routed through
// the VPN tunnel. On Android, this calls VpnService.protect(fd).
var socketProtector func(fd int32) bool

// defaultIfaceMu protects defaultIfaceListener and pendingIfaceUpdate.
var defaultIfaceMu sync.Mutex

var (
	defaultIfaceListener libbox.InterfaceUpdateListener
	pendingIfaceUpdate   *ifaceUpdate
)

type ifaceUpdate struct {
	name      string
	index     int32
	expensive bool
}

// cachedInterfacesJSON stores interface data provided by the mobile platform.
// On Android, net.Interfaces() fails (netlink permission denied), so the Kotlin
// side enumerates interfaces via ConnectivityManager and passes JSON here.
var (
	cachedInterfacesJSON string
	cachedInterfacesMu   sync.RWMutex
)

// SetInterfacesJSON stores interface data from the mobile platform.
func SetInterfacesJSON(json string) {
	cachedInterfacesMu.Lock()
	defer cachedInterfacesMu.Unlock()
	cachedInterfacesJSON = json
	slog.Info("set interfaces JSON", "len", len(json))
}

// UpdateDefaultInterface is called from the mobile side (via FFI) to report
// the current default network interface detected via ConnectivityManager.
func UpdateDefaultInterface(name string, index int64, expensive bool) {
	defaultIfaceMu.Lock()
	defer defaultIfaceMu.Unlock()
	slog.Info("[DIAG] UpdateDefaultInterface", "name", name, "index", index, "expensive", expensive, "hasListener", defaultIfaceListener != nil)
	upd := &ifaceUpdate{name: name, index: int32(index), expensive: expensive}
	if defaultIfaceListener != nil {
		slog.Info("[DIAG] UpdateDefaultInterface: calling listener.UpdateDefaultInterface")
		defaultIfaceListener.UpdateDefaultInterface(upd.name, upd.index, upd.expensive, false)
	} else {
		slog.Info("[DIAG] UpdateDefaultInterface: storing pending update (listener not yet registered)")
		pendingIfaceUpdate = upd
	}
}

// SetSocketProtector registers a callback that protects socket fds from VPN routing.
func SetSocketProtector(fn func(fd int32) bool) {
	slog.Info("set socket protector", "registered", fn != nil)
	socketProtector = fn
}

// PlatformIO implements libbox.PlatformInterface.
type PlatformIO struct {
	mu          sync.RWMutex
	tunFd       int32
	externalTun bool
}

// SetTunFd stores a TUN file descriptor from VpnService (Android)
// or NetworkExtension (iOS).
func (p *PlatformIO) SetTunFd(fd int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tunFd = fd
	if fd != 0 {
		p.externalTun = true
	}
	slog.Debug("set TUN fd", "fd", fd, "externalTun", fd != 0)
}

// ResetTunFd clears the stored TUN file descriptor.
func (p *PlatformIO) ResetTunFd() {
	p.mu.Lock()
	defer p.mu.Unlock()
	oldFd := p.tunFd
	p.tunFd = 0
	p.externalTun = false
	slog.Debug("reset TUN fd", "cleared", oldFd)
}

func (p *PlatformIO) LocalDNSTransport() libbox.LocalDNSTransport {
	return nil
}

func (p *PlatformIO) UsePlatformAutoDetectInterfaceControl() bool {
	slog.Info("[DIAG] UsePlatformAutoDetectInterfaceControl → true")
	return true
}

func (p *PlatformIO) AutoDetectInterfaceControl(fd int32) error {
	if socketProtector != nil {
		ok := socketProtector(fd)
		slog.Info("[DIAG] AutoDetectInterfaceControl", "fd", fd, "protected", ok)
		if !ok {
			return fmt.Errorf("protect fd %d failed", fd)
		}
	} else {
		slog.Warn("[DIAG] AutoDetectInterfaceControl: no socket protector registered", "fd", fd)
	}
	return nil
}

// OpenTun returns the external TUN fd and consumes it.
func (p *PlatformIO) OpenTun(options libbox.TunOptions) (int32, error) {
	p.mu.Lock()
	fd := p.tunFd
	if fd != 0 {
		p.tunFd = 0
	}
	p.mu.Unlock()
	slog.Info("[DIAG] OpenTun called", "storedFd", fd, "fdValid", fd != 0)
	if fd != 0 {
		return fd, nil
	}
	slog.Error("[DIAG] OpenTun: no TUN fd available")
	return 0, os.ErrInvalid
}

func (p *PlatformIO) UseProcFS() bool {
	return false
}

func (p *PlatformIO) FindConnectionOwner(ipProtocol int32, sourceAddress string, sourcePort int32, destinationAddress string, destinationPort int32) (*libbox.ConnectionOwner, error) {
	return nil, os.ErrInvalid
}

func (p *PlatformIO) StartDefaultInterfaceMonitor(listener libbox.InterfaceUpdateListener) error {
	defaultIfaceMu.Lock()
	defer defaultIfaceMu.Unlock()
	defaultIfaceListener = listener
	slog.Info("[DIAG] StartDefaultInterfaceMonitor", "hasPendingUpdate", pendingIfaceUpdate != nil)
	if pendingIfaceUpdate != nil {
		slog.Info("applying pending interface update", "name", pendingIfaceUpdate.name, "index", pendingIfaceUpdate.index, "expensive", pendingIfaceUpdate.expensive)
		listener.UpdateDefaultInterface(pendingIfaceUpdate.name, pendingIfaceUpdate.index, pendingIfaceUpdate.expensive, false)
		pendingIfaceUpdate = nil
	} else if runtime.GOOS != "android" && runtime.GOOS != "ios" {
		// Desktop: use UDP dial detection
		go detectDefaultInterface(listener)
	} else {
		slog.Info("[DIAG] StartDefaultInterfaceMonitor: no pending update, waiting for UpdateDefaultInterface call")
	}
	return nil
}

func (p *PlatformIO) CloseDefaultInterfaceMonitor(listener libbox.InterfaceUpdateListener) error {
	defaultIfaceMu.Lock()
	defaultIfaceListener = nil
	defaultIfaceMu.Unlock()
	slog.Debug("closing interface monitor")
	return nil
}

// detectDefaultInterface attempts to find the default network interface by
// dialing UDP to public DNS servers.
func detectDefaultInterface(listener libbox.InterfaceUpdateListener) {
	if listener == nil {
		return
	}
	targets := []string{"8.8.8.8:53", "1.1.1.1:53"}
	for attempt := 0; attempt < 5; attempt++ {
		for _, target := range targets {
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
		slog.Warn("detect interface: attempt failed, retrying", "attempt", attempt+1, "wait", attempt+1)
		time.Sleep(time.Duration(attempt+1) * time.Second)
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
		slog.Error("parse interfaces JSON failed", "error", err, "json", jsonStr[:min(200, len(jsonStr))])
		return nil, fmt.Errorf("parse interfaces JSON: %w", err)
	}
	var result []*libbox.NetworkInterface
	for _, mi := range ifaces {
		slog.Info("[DIAG] parsed interface", "name", mi.Name, "index", mi.Index, "flags", mi.Flags, "addrs", len(mi.Addresses))
		result = append(result, &libbox.NetworkInterface{
			Index:     mi.Index,
			MTU:       mi.MTU,
			Name:      mi.Name,
			Addresses: &stringIterator{items: mi.Addresses},
			Flags:     mi.Flags,
			Type:      mi.Type,
		})
	}
	slog.Info("parsed mobile interfaces", "count", len(result))
	return &networkInterfaceIterator{items: result}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (p *PlatformIO) GetInterfaces() (libbox.NetworkInterfaceIterator, error) {
	cachedInterfacesMu.RLock()
	jsonStr := cachedInterfacesJSON
	cachedInterfacesMu.RUnlock()
	slog.Info("[DIAG] GetInterfaces", "hasCachedJSON", jsonStr != "", "jsonLen", len(jsonStr))
	if jsonStr != "" {
		return parseInterfacesJSON(jsonStr)
	}

	// Desktop fallback: use Go's net.Interfaces()
	slog.Warn("[DIAG] GetInterfaces: no cached JSON, using net.Interfaces() fallback")
	ifaces, err := net.Interfaces()
	if err != nil {
		slog.Error("get interfaces failed", "error", err)
		return nil, err
	}
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

		// Build raw flags compatible with linkFlags() in libbox.
		// Use numeric constants instead of syscall.IFF_* for Windows compatibility.
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

func (p *PlatformIO) UnderNetworkExtension() bool {
	if runtime.GOOS != "ios" {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.externalTun
}

func (p *PlatformIO) IncludeAllNetworks() bool {
	return runtime.GOOS == "ios"
}

func (p *PlatformIO) ReadWIFIState() *libbox.WIFIState {
	return nil
}

func (p *PlatformIO) SystemCertificates() libbox.StringIterator {
	return nil
}

func (p *PlatformIO) ClearDNSCache() {}

func (p *PlatformIO) SendNotification(notification *libbox.Notification) error {
	return nil
}

// stringIterator implements libbox.StringIterator.
type stringIterator struct {
	items []string
	index int
}

func (s *stringIterator) Len() int32 {
	return int32(len(s.items))
}

func (s *stringIterator) Next() string {
	if s.index >= len(s.items) {
		return ""
	}
	item := s.items[s.index]
	s.index++
	return item
}

func (s *stringIterator) HasNext() bool {
	return s.index < len(s.items)
}

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

func (it *networkInterfaceIterator) HasNext() bool {
	return it.index < len(it.items)
}
