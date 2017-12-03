// udp
package main

import (
	"net"
	"syscall"
)

type Multicast struct {
	conn    *net.UDPConn
	udpAddr *net.UDPAddr
	sysconn syscall.RawConn
}

func NewMulticastListener(address string, netInterface *net.Interface) (*Multicast, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenMulticastUDP("udp", netInterface, udpAddr)
	if err != nil {
		return nil, err
	}

	sysconn, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}

	c := &Multicast{
		conn,
		udpAddr,
		sysconn,
	}
	return c, nil
}

func NewMulticastSender(address string, netInterface *net.Interface) (*Multicast, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, err
	}

	sysconn, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}

	c := &Multicast{
		conn,
		udpAddr,
		sysconn,
	}
	return c, nil
}

func (c *Multicast) SetDatagramSize(datagramSize int) error {
	err := c.conn.SetReadBuffer(datagramSize)
	if err != nil {
		return err
	}
	return c.conn.SetWriteBuffer(datagramSize)
}

func (c *Multicast) SetTTL(ttl int) error {
	return c.sysconn.Control(func(fd uintptr) {
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, ttl)
	})
}

func (c *Multicast) SetLoopback(enable bool) error {
	return c.sysconn.Control(func(fd uintptr) {
		lp := 0
		if enable {
			lp = -1
		}
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, lp)
	})
}
