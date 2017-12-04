// udp
package main

import (
	"net"
	"syscall"
)

type Multicast struct {
	controlConn *net.UDPConn
	controlAddr *net.UDPAddr
	dataConn    *net.UDPConn
	dataAddr    *net.UDPAddr
}

func NewMulticast(address string, netInterface *net.Interface) (*Multicast, error) {
	controlAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	controlConn, err := net.ListenMulticastUDP("udp", netInterface, controlAddr)
	if err != nil {
		return nil, err
	}

	// Data address is port+1:
	dataAddr := &net.UDPAddr{
		IP:   controlAddr.IP,
		Port: controlAddr.Port + 1,
		Zone: controlAddr.Zone,
	}

	dataConn, err := net.ListenMulticastUDP("udp", netInterface, dataAddr)
	if err != nil {
		return nil, err
	}

	c := &Multicast{
		controlConn,
		controlAddr,
		dataConn,
		dataAddr,
	}
	return c, nil
}

func (m *Multicast) SetDatagramSize(datagramSize int) error {
	err := m.controlConn.SetReadBuffer(datagramSize)
	if err != nil {
		return err
	}
	err = m.controlConn.SetWriteBuffer(datagramSize)
	if err != nil {
		return err
	}
	err = m.dataConn.SetReadBuffer(datagramSize)
	if err != nil {
		return err
	}
	err = m.dataConn.SetWriteBuffer(datagramSize)
	if err != nil {
		return err
	}
	return nil
}

func (m *Multicast) SetTTL(ttl int) error {
	err := setSocketOptionInt(m.controlConn, syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, ttl)
	if err != nil {
		return err
	}
	err = setSocketOptionInt(m.dataConn, syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, ttl)
	if err != nil {
		return err
	}
	return nil
}

func (m *Multicast) SetLoopback(enable bool) error {
	lp := 0
	if enable {
		lp = -1
	}
	err := setSocketOptionInt(m.controlConn, syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, lp)
	if err != nil {
		return err
	}
	err = setSocketOptionInt(m.dataConn, syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, lp)
	if err != nil {
		return err
	}
	return nil
}

func (m *Multicast) SendControl(msg []byte) (int, error) {
	return m.controlConn.WriteToUDP(msg, m.controlAddr)
}

func (m *Multicast) SendData(msg []byte) (int, error) {
	return m.dataConn.WriteToUDP(msg, m.dataAddr)
}

func (m *Multicast) RecvControl(msg []byte) (int, error) {
	return m.controlConn.Read(msg)
}

func (m *Multicast) RecvData(msg []byte) (int, error) {
	return m.dataConn.Read(msg)
}
