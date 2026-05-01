package core

import (
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
	if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		// Trigger UpdateInterfaces() to populate network interface cache,
		// but pass index=-1 so DefaultInterface() stays nil. This ensures
		// selectInterfaces() uses all available (non-TUN) interfaces.
		listener.UpdateDefaultInterface("", -1, false, false)
		slog.Info("[DIAG] StartDefaultInterfaceMonitor: triggered interface update (mobile)")
		return nil
	}
	slog.Info("starting interface detection")
	go detectDefaultInterface(listener)
	return nil
}

func (p *PlatformIO) CloseDefaultInterfaceMonitor(listener libbox.InterfaceUpdateListener) error {
	slog.Debug("closing interface monitor")
	return nil
}

// detectDefaultInterface attempts to find the default network interface by
// dialing UDP to public DNS servers.
func detectDefaultInterface(listener libbox.InterfaceUpdateListener) {
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

func (p *PlatformIO) GetInterfaces() (libbox.NetworkInterfaceIterator, error) {
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
		result = append(result, &libbox.NetworkInterface{
			Index:     int32(iface.Index),
			MTU:       int32(iface.MTU),
			Name:      iface.Name,
			Addresses: &stringIterator{items: addrStrings},
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
