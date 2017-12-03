// udp
package main

import (
	"net"
)

type Client {
	s *net.UDPConn
	TTL int
	Loopback bool
}

func NewClient() (*Client, error) {
	c := &Client{
		s: s,
	}


	// Set advanced options for TTL and loopback:
	sysconn, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	sysconn.Control(func(fd uintptr) {
		lp := 0
		if loopbackEnable {
			lp = -1
		}
		syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, ttl)
		syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, lp)
	})
	conn.SetReadBuffer(datagramSize)

}
