//go:build !android && !ios

package core

func dupFd(fd int32) (int32, error) { return fd, nil }

func closeFd(_ int32) error { return nil }
