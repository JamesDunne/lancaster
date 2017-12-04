// udp
package main

import (
	"net"
	"syscall"
)

type UDPMessage struct {
	Error error

	Data          []byte
	SourceAddress *net.UDPAddr
}

type Multicast struct {
	controlConn  *net.UDPConn
	controlAddr  *net.UDPAddr
	dataConn     *net.UDPConn
	dataAddr     *net.UDPAddr
	datagramSize int

	Control chan UDPMessage
	Data    chan UDPMessage
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
		1500,
		make(chan UDPMessage),
		make(chan UDPMessage),
	}
	return c, nil
}

func (m *Multicast) SetDatagramSize(datagramSize int) error {
	m.datagramSize = datagramSize
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

func (m *Multicast) receiveLoop(conn *net.UDPConn, ch chan UDPMessage) {
	// Start a message receive loop:
	for {
		buf := make([]byte, m.datagramSize)
		n, recvAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			ch <- UDPMessage{Error: err}
			return
		}
		ch <- UDPMessage{Data: buf[0:n], SourceAddress: recvAddr}
	}
}

func (m *Multicast) SendControl(msg []byte) (int, error) {
	return m.controlConn.WriteToUDP(msg, m.controlAddr)
}

func (m *Multicast) ControlReceiveLoop() {
	m.receiveLoop(m.controlConn, m.Control)
}

func (m *Multicast) SendData(msg []byte) (int, error) {
	return m.dataConn.WriteToUDP(msg, m.dataAddr)
}

func (m *Multicast) DataReceiveLoop() {
	m.receiveLoop(m.dataConn, m.Data)
}
