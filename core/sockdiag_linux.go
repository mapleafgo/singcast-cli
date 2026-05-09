//go:build linux

package core

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/sagernet/sing-box/adapter"
)

const (
	sockDiagByFamily        = 20
	sockDiagResponseMinSize = 72
)

var netlinkUnavailable bool

var sockDiagPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 64<<10)
		return &buf
	},
}

func findConnectionOwnerImpl(ipProtocol int32, srcAddr string, srcPort int32, dstAddr string, dstPort int32) (*adapter.ConnectionOwner, error) {
	srcIP := net.ParseIP(srcAddr)
	dstIP := net.ParseIP(dstAddr)
	if srcIP == nil || dstIP == nil {
		return nil, os.ErrInvalid
	}

	family := uint8(syscall.AF_INET)
	protocol := uint8(syscall.IPPROTO_TCP)
	if ipProtocol == 17 {
		protocol = uint8(syscall.IPPROTO_UDP)
	}
	if srcIP.To4() == nil {
		family = uint8(syscall.AF_INET6)
	}

	uid, inode, err := querySockDiag(family, protocol, srcIP, uint16(srcPort), dstIP, uint16(dstPort))
	if err != nil {
		return nil, err
	}
	if inode == 0 {
		return nil, os.ErrInvalid
	}

	processPath, pathErr := resolveProcessPathByInode(inode, uid)
	if pathErr != nil {
		return &adapter.ConnectionOwner{UserId: int32(uid)}, nil
	}

	return &adapter.ConnectionOwner{
		UserId:      int32(uid),
		ProcessPath: processPath,
	}, nil
}

func querySockDiag(family, protocol uint8, srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16) (uid, inode uint32, err error) {
	if netlinkUnavailable {
		return 0, 0, fmt.Errorf("open netlink: unavailable")
	}

	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, syscall.NETLINK_INET_DIAG)
	if err != nil {
		netlinkUnavailable = true
		return 0, 0, fmt.Errorf("open netlink: %w", err)
	}
	defer syscall.Close(fd)

	timeout := &syscall.Timeval{Usec: 100}
	syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, timeout)
	syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, timeout)

	if err := syscall.Connect(fd, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return 0, 0, fmt.Errorf("connect netlink: %w", err)
	}

	req := packSockDiagRequest(family, protocol, srcIP, srcPort, dstIP, dstPort)
	if _, err := syscall.Write(fd, req[:]); err != nil {
		return 0, 0, fmt.Errorf("write netlink: %w", err)
	}

	bufPtr := sockDiagPool.Get().(*[]byte)
	defer sockDiagPool.Put(bufPtr)
	buf := *bufPtr

	n, err := syscall.Read(fd, buf)
	if err != nil {
		return 0, 0, fmt.Errorf("read netlink: %w", err)
	}

	msgs, err := syscall.ParseNetlinkMessage(buf[:n])
	if err != nil {
		return 0, 0, fmt.Errorf("parse netlink: %w", err)
	}

	for _, msg := range msgs {
		switch msg.Header.Type {
		case sockDiagByFamily:
			if len(msg.Data) >= sockDiagResponseMinSize {
				uid = binary.NativeEndian.Uint32(msg.Data[64:68])
				inode = binary.NativeEndian.Uint32(msg.Data[68:72])
				return uid, inode, nil
			}
		case syscall.NLMSG_ERROR:
			return 0, 0, fmt.Errorf("netlink error response")
		}
	}

	return 0, 0, os.ErrInvalid
}

func packSockDiagRequest(family, protocol uint8, srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16) [72]byte {
	var req [72]byte

	binary.NativeEndian.PutUint32(req[0:4], 72)
	binary.NativeEndian.PutUint16(req[4:6], sockDiagByFamily)
	binary.NativeEndian.PutUint16(req[6:8], syscall.NLM_F_REQUEST)

	req[16] = family
	req[17] = protocol
	binary.NativeEndian.PutUint32(req[20:24], 0x02)

	binary.BigEndian.PutUint16(req[24:26], srcPort)
	binary.BigEndian.PutUint16(req[26:28], dstPort)

	if v4 := srcIP.To4(); v4 != nil {
		copy(req[28:32], v4)
	} else {
		copy(req[28:44], srcIP.To16())
	}

	if v4 := dstIP.To4(); v4 != nil {
		copy(req[44:48], v4)
	} else {
		copy(req[44:60], dstIP.To16())
	}

	binary.NativeEndian.PutUint32(req[64:68], 0xFFFFFFFF)
	binary.NativeEndian.PutUint32(req[68:72], 0xFFFFFFFF)

	return req
}

func resolveProcessPathByInode(targetInode, uid uint32) (string, error) {
	procEntries, err := os.ReadDir("/proc")
	if err != nil {
		return "", os.ErrInvalid
	}

	target := fmt.Sprintf("socket:[%d]", targetInode)

	for _, entry := range procEntries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		stat, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
		if err != nil {
			continue
		}
		if sysStat, ok := stat.Sys().(*syscall.Stat_t); !ok || sysStat.Uid != uid {
			continue
		}

		fdPath := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fdPath)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdPath, fd.Name()))
			if err != nil {
				continue
			}
			if link == target {
				exe, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
				return exe, nil
			}
		}
	}

	return "", os.ErrInvalid
}
