//go:build linux

package core

import (
	"encoding/binary"
	"net"
	"syscall"
	"testing"
)

func TestPackSockDiagRequest_IPv4TCP(t *testing.T) {
	srcIP := net.ParseIP("192.168.1.100")
	dstIP := net.ParseIP("10.0.0.1")
	req := packSockDiagRequest(uint8(syscall.AF_INET), uint8(syscall.IPPROTO_TCP), srcIP, 12345, dstIP, 80)

	// Check total length.
	if len(req) != 72 {
		t.Fatalf("request length = %d, want 72", len(req))
	}

	// nlmsghdr.len
	if got := binary.NativeEndian.Uint32(req[0:4]); got != 72 {
		t.Errorf("nlmsghdr.len = %d, want 72", got)
	}

	// nlmsghdr.type
	if got := binary.NativeEndian.Uint16(req[4:6]); got != sockDiagByFamily {
		t.Errorf("nlmsghdr.type = %d, want %d", got, sockDiagByFamily)
	}

	// nlmsghdr.flags
	if got := binary.NativeEndian.Uint16(req[6:8]); got != syscall.NLM_F_REQUEST {
		t.Errorf("nlmsghdr.flags = %d, want NLM_F_REQUEST", got)
	}

	// family
	if req[16] != uint8(syscall.AF_INET) {
		t.Errorf("family = %d, want AF_INET", req[16])
	}

	// protocol
	if req[17] != uint8(syscall.IPPROTO_TCP) {
		t.Errorf("protocol = %d, want IPPROTO_TCP", req[17])
	}

	// states
	if got := binary.NativeEndian.Uint32(req[20:24]); got != 0x02 {
		t.Errorf("states = %d, want 0x02 (TCP_ESTABLISHED)", got)
	}

	// source port (big-endian)
	if got := binary.BigEndian.Uint16(req[24:26]); got != 12345 {
		t.Errorf("src port = %d, want 12345", got)
	}

	// destination port (big-endian)
	if got := binary.BigEndian.Uint16(req[26:28]); got != 80 {
		t.Errorf("dst port = %d, want 80", got)
	}

	// source IP
	if !net.IP(req[28:32]).Equal(srcIP.To4()) {
		t.Errorf("src IP = %v, want %v", net.IP(req[28:32]), srcIP)
	}

	// destination IP
	if !net.IP(req[44:48]).Equal(dstIP.To4()) {
		t.Errorf("dst IP = %v, want %v", net.IP(req[44:48]), dstIP)
	}

	// cookie
	if binary.NativeEndian.Uint32(req[64:68]) != 0xFFFFFFFF {
		t.Error("cookie[0] should be 0xFFFFFFFF")
	}
	if binary.NativeEndian.Uint32(req[68:72]) != 0xFFFFFFFF {
		t.Error("cookie[1] should be 0xFFFFFFFF")
	}
}

func TestPackSockDiagRequest_IPv6(t *testing.T) {
	srcIP := net.ParseIP("::1")
	dstIP := net.ParseIP("fe80::1")
	req := packSockDiagRequest(uint8(syscall.AF_INET6), uint8(syscall.IPPROTO_TCP), srcIP, 443, dstIP, 80)

	if req[16] != uint8(syscall.AF_INET6) {
		t.Errorf("family = %d, want AF_INET6", req[16])
	}

	// IPv6 source address at offset 28:44
	if !net.IP(req[28:44]).Equal(srcIP.To16()) {
		t.Errorf("src IPv6 mismatch")
	}

	// IPv6 destination address at offset 44:60
	if !net.IP(req[44:60]).Equal(dstIP.To16()) {
		t.Errorf("dst IPv6 mismatch")
	}
}

func TestFindConnectionOwnerImpl_InvalidIP(t *testing.T) {
	_, err := findConnectionOwnerImpl(6, "not-an-ip", 1234, "10.0.0.1", 80)
	if err == nil {
		t.Error("expected error for invalid source IP")
	}

	_, err = findConnectionOwnerImpl(6, "192.168.1.1", 1234, "not-an-ip", 80)
	if err == nil {
		t.Error("expected error for invalid destination IP")
	}
}

func TestPackSockDiagRequest_UDP(t *testing.T) {
	srcIP := net.ParseIP("192.168.1.1")
	dstIP := net.ParseIP("8.8.8.8")
	req := packSockDiagRequest(uint8(syscall.AF_INET), uint8(syscall.IPPROTO_UDP), srcIP, 54321, dstIP, 53)

	if req[17] != uint8(syscall.IPPROTO_UDP) {
		t.Errorf("protocol = %d, want IPPROTO_UDP", req[17])
	}
}

func TestSockDiagPool(t *testing.T) {
	bufPtr := sockDiagPool.Get().(*[]byte)
	buf := *bufPtr
	if len(buf) != 64<<10 {
		t.Errorf("pool buffer len = %d, want %d", len(buf), 64<<10)
	}
	sockDiagPool.Put(bufPtr)

	// Get again should return the same buffer (recycled).
	bufPtr2 := sockDiagPool.Get().(*[]byte)
	if len(*bufPtr2) != 64<<10 {
		t.Errorf("recycled pool buffer len = %d, want %d", len(*bufPtr2), 64<<10)
	}
	sockDiagPool.Put(bufPtr2)
}
