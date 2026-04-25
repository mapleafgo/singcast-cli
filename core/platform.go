package core

import (
	"os"

	"github.com/sagernet/sing-box/experimental/libbox"
)

// PlatformIO implements libbox.PlatformInterface with no-op methods.
type PlatformIO struct{}

func (p *PlatformIO) LocalDNSTransport() libbox.LocalDNSTransport {
	return nil
}

func (p *PlatformIO) UsePlatformAutoDetectInterfaceControl() bool {
	return false
}

func (p *PlatformIO) AutoDetectInterfaceControl(fd int32) error {
	return nil
}

func (p *PlatformIO) OpenTun(options libbox.TunOptions) (int32, error) {
	return 0, os.ErrInvalid
}

func (p *PlatformIO) UseProcFS() bool {
	return false
}

func (p *PlatformIO) FindConnectionOwner(ipProtocol int32, sourceAddress string, sourcePort int32, destinationAddress string, destinationPort int32) (*libbox.ConnectionOwner, error) {
	return nil, os.ErrInvalid
}

func (p *PlatformIO) StartDefaultInterfaceMonitor(listener libbox.InterfaceUpdateListener) error {
	return nil
}

func (p *PlatformIO) CloseDefaultInterfaceMonitor(listener libbox.InterfaceUpdateListener) error {
	return nil
}

func (p *PlatformIO) GetInterfaces() (libbox.NetworkInterfaceIterator, error) {
	return nil, os.ErrInvalid
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
