//go:build windows

package server

import "syscall"

func setTCPSocketOptions(fd uintptr, recvBuf, sendBuf int) error {
	h := syscall.Handle(fd)
	if err := syscall.SetsockoptInt(h, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1); err != nil {
		return err
	}
	if recvBuf > 0 {
		if err := syscall.SetsockoptInt(h, syscall.SOL_SOCKET, syscall.SO_RCVBUF, recvBuf); err != nil {
			return err
		}
	}
	if sendBuf > 0 {
		if err := syscall.SetsockoptInt(h, syscall.SOL_SOCKET, syscall.SO_SNDBUF, sendBuf); err != nil {
			return err
		}
	}
	return nil
}
