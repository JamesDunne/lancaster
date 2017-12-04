// udp
package main

import (
	"net"
	"syscall"
)

type Multicast struct {
	listenConn    *net.UDPConn
	listenSysConn syscall.RawConn

	GroupAddress *net.UDPAddr
}

func NewMulticast(address string, netInterface *net.Interface) (*Multicast, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	listenConn, err := net.ListenMulticastUDP("udp", netInterface, udpAddr)
	if err != nil {
		return nil, err
	}

	listenSysConn, err := listenConn.SyscallConn()
	if err != nil {
		return nil, err
	}

	c := &Multicast{
		listenConn,
		listenSysConn,
		udpAddr,
	}
	return c, nil
}

func (c *Multicast) SetDatagramSize(datagramSize int) error {
	err := c.listenConn.SetReadBuffer(datagramSize)
	if err != nil {
		return err
	}
	err = c.listenConn.SetWriteBuffer(datagramSize)
	if err != nil {
		return err
	}
	return nil
}

func (c *Multicast) Send(msg []byte) (int, error) {
	return c.listenConn.WriteToUDP(msg, c.GroupAddress)
}
