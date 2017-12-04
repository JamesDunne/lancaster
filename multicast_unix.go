// +build darwin dragonfly freebsd linux netbsd openbsd solaris

package main

import (
	"syscall"
)

func (c *Multicast) SetTTL(ttl int) error {
	return c.listenSysConn.Control(func(fd uintptr) {
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, ttl)
	})
}

func (c *Multicast) SetLoopback(enable bool) error {
	return c.listenSysConn.Control(func(fd uintptr) {
		lp := 0
		if enable {
			lp = -1
		}
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, lp)
	})
}
