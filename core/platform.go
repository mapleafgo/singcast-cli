package core

import (
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/sagernet/sing-box/experimental/libbox"
)

// PlatformIO implements libbox.PlatformInterface.
// On desktop, it provides default implementations for all methods.
// On mobile, callers can set a TUN fd via SetTunFd before starting
// a configuration that contains a TUN inbound.
type PlatformIO struct {
	mu    sync.RWMutex
	tunFd int32
}

// SetTunFd stores a TUN file descriptor from VpnService (Android)
// or NetworkExtension (iOS). OpenTun returns this fd when set.
func (p *PlatformIO) SetTunFd(fd int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tunFd = fd
}

// ResetTunFd clears the stored TUN file descriptor. Called when the
// service stops so that a stale fd cannot be reused accidentally.
func (p *PlatformIO) ResetTunFd() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tunFd = 0
}

func (p *PlatformIO) LocalDNSTransport() libbox.LocalDNSTransport {
	return nil
}

func (p *PlatformIO) UsePlatformAutoDetectInterfaceControl() bool {
	return true
}

func (p *PlatformIO) AutoDetectInterfaceControl(fd int32) error {
	return nil
}

func (p *PlatformIO) OpenTun(options libbox.TunOptions) (int32, error) {
	p.mu.RLock()
	fd := p.tunFd
	p.mu.RUnlock()
	if fd != 0 {
		return fd, nil
	}
	return 0, os.ErrInvalid
}

func (p *PlatformIO) UseProcFS() bool {
	return false
}

func (p *PlatformIO) FindConnectionOwner(ipProtocol int32, sourceAddress string, sourcePort int32, destinationAddress string, destinationPort int32) (*libbox.ConnectionOwner, error) {
	return nil, os.ErrInvalid
}

func (p *PlatformIO) StartDefaultInterfaceMonitor(listener libbox.InterfaceUpdateListener) error {
	conn, err := net.Dial("udp4", "8.8.8.8:53")
	if err != nil {
		return err
	}
	localAddr := conn.LocalAddr()
	conn.Close()

	localUDP, ok := localAddr.(*net.UDPAddr)
	if !ok {
		return fmt.Errorf("unexpected local address type: %T", localAddr)
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return err
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
				listener.UpdateDefaultInterface(iface.Name, int32(iface.Index), false, false)
				return nil
			}
		}
	}
	return nil
}

func (p *PlatformIO) CloseDefaultInterfaceMonitor(listener libbox.InterfaceUpdateListener) error {
	return nil
}

func (p *PlatformIO) GetInterfaces() (libbox.NetworkInterfaceIterator, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
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
	return &networkInterfaceIterator{items: result}, nil
}

func (p *PlatformIO) UnderNetworkExtension() bool {
	return false
}

func (p *PlatformIO) IncludeAllNetworks() bool {
	return false
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
