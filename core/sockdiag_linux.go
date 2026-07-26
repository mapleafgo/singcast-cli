//go:build linux

package core

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sagernet/sing-box/adapter"
)

// 以下常量对应内核 linux/inet_diag.h 的结构布局，改动前须核对内核头文件。
const (
	// sockDiagByFamily 是 SOCK_DIAG_BY_FAMILY 消息类型。
	sockDiagByFamily = 20
	// sockDiagResponseMinSize 是 inet_diag_msg 的最小长度（读 idiag_inode 需覆盖到 72 字节）。
	sockDiagResponseMinSize = 72
	// idiagUIDOffset/idiagInodeOffset 是 inet_diag_msg 中 idiag_uid 与 idiag_inode 的字节偏移。
	idiagUIDOffset   = 64
	idiagInodeOffset = 68
	// tcpStateEstablished 是 idiag_states 位图里 TCP_ESTABLISHED 对应的位（1 << 1）。
	tcpStateEstablished = 0x02
	// inetDiagNoCookie 表示不按 socket cookie 过滤（INET_DIAG_NOCOOKIE）。
	inetDiagNoCookie = 0xFFFFFFFF

	// sockDiagTimeout 是单次 netlink 收发的上限。内核应答通常在微秒级，
	// 但取值需高于一个调度时间片，否则系统稍有负载就随机超时，
	// 表现为基于进程归属的路由规则命中不稳定。
	sockDiagTimeout = 100 * time.Millisecond

	// slowInodeScanThreshold 是 /proc 扫描打慢日志的阈值。取 50ms：
	// 正常情况远低于此，超过就意味着给每条新连接都加了可感知的建连延迟。
	slowInodeScanThreshold = 50 * time.Millisecond
)

// netlinkUnavailable 在确认内核不支持 NETLINK_INET_DIAG 后闩锁，跳过后续无谓的
// socket 调用。只对"确定不支持"的 errno 置位——瞬时错误（如 fd 耗尽 EMFILE）
// 若也闩锁，一次抖动就会永久禁用连接归属查询直到进程重启。
var netlinkUnavailable atomic.Bool

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

	// 进程路径解析失败不视为致命错误：能拿到 uid 就返回部分结果，
	// 调用方至少可以用 uid 做路由判断。
	processPath, pathErr := resolveProcessPathByInode(inode, uid) //nolint:nilerr // 降级返回 uid，不传播错误
	if pathErr != nil {
		return &adapter.ConnectionOwner{UserId: int32(uid)}, nil //nolint:nilerr
	}

	return &adapter.ConnectionOwner{
		UserId:      int32(uid),
		ProcessPath: processPath,
	}, nil
}

// isNetlinkUnsupported 判断错误是否表示内核/环境确定不提供 NETLINK_INET_DIAG，
// 而非可恢复的瞬时失败。容器中该协议常被 seccomp 挡掉（EPERM/EAFNOSUPPORT）。
func isNetlinkUnsupported(err error) bool {
	return errors.Is(err, syscall.EPROTONOSUPPORT) ||
		errors.Is(err, syscall.EAFNOSUPPORT) ||
		errors.Is(err, syscall.EPERM) ||
		errors.Is(err, syscall.ENOSYS)
}

func querySockDiag(family, protocol uint8, srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16) (uid, inode uint32, err error) {
	if netlinkUnavailable.Load() {
		return 0, 0, fmt.Errorf("open netlink: unavailable")
	}

	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, syscall.NETLINK_INET_DIAG)
	if err != nil {
		if isNetlinkUnsupported(err) {
			netlinkUnavailable.Store(true)
			slog.Warn("netlink inet_diag unsupported, process-based routing disabled", "error", err)
		}
		return 0, 0, fmt.Errorf("open netlink: %w", err)
	}
	defer syscall.Close(fd)

	timeout := &syscall.Timeval{Usec: sockDiagTimeout.Microseconds()}
	_ = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, timeout)
	_ = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, timeout)

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
				// inet_diag_msg 尾部两个字段（linux/inet_diag.h）：
				// offset 64 = idiag_uid (__u32)、offset 68 = idiag_inode (__u32)。
				uid = binary.NativeEndian.Uint32(msg.Data[idiagUIDOffset : idiagUIDOffset+4])
				inode = binary.NativeEndian.Uint32(msg.Data[idiagInodeOffset : idiagInodeOffset+4])
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
	binary.NativeEndian.PutUint32(req[20:24], tcpStateEstablished)

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

	binary.NativeEndian.PutUint32(req[64:68], inetDiagNoCookie)
	binary.NativeEndian.PutUint32(req[68:72], inetDiagNoCookie)

	return req
}

// resolveProcessPathByInode 通过扫描 /proc/*/fd 找出持有该 socket inode 的进程可执行路径。
// 复杂度是 O(进程数 × fd 数)，每条新连接都会走一次，因此对超过
// slowInodeScanThreshold 的扫描打 Debug 日志——否则"代理变慢"无从定位到这里。
func resolveProcessPathByInode(targetInode, uid uint32) (string, error) {
	start := time.Now()
	defer func() {
		if elapsed := time.Since(start); elapsed > slowInodeScanThreshold {
			slog.Debug("slow /proc inode scan",
				"inode", targetInode, "uid", uid, "elapsed_ms", elapsed.Milliseconds())
		}
	}()

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
