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
