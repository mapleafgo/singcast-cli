//go:build android || ios

package core

import "syscall"

func dupFd(fd int32) (int32, error) {
	n, err := syscall.Dup(int(fd))
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

func closeFd(fd int32) error { return syscall.Close(int(fd)) }
