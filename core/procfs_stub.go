//go:build !linux

package core

import (
	"os"

	"github.com/sagernet/sing-box/experimental/libbox"
)

func findConnectionOwnerImpl(ipProtocol int32, sourceAddress string, sourcePort int32, destinationAddress string, destinationPort int32) (*libbox.ConnectionOwner, error) {
	return nil, os.ErrInvalid
}
