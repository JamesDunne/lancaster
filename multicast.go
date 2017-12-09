// udp
package main

import (
	"net"
	"syscall"
)

// Data messages:
const (
	_ = iota
	MetadataSection
	DataSection
)

type UDPMessage struct {
	Error error

	Data          []byte
	SourceAddress *net.UDPAddr
}

type Multicast struct {
	netInterface      *net.Interface
	datagramSize      int
	bufferPacketCount int
	ttl               int
	loopback          bool

	controlToServerAddr *net.UDPAddr
	controlToClientAddr *net.UDPAddr
	dataAddr            *net.UDPAddr

	controlToServerConn *net.UDPConn
	controlToClientConn *net.UDPConn
	dataConn            *net.UDPConn

	ControlToServer chan UDPMessage
	ControlToClient chan UDPMessage
	Data            chan UDPMessage
}

func NewMulticast(address string, netInterface *net.Interface) (*Multicast, error) {
	// Control to-server address is port+0:
	controlToServerAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	// Control to-client address is port+1:
	controlToClientAddr := &net.UDPAddr{
		IP:   controlToServerAddr.IP,
		Port: controlToServerAddr.Port + 1,
		Zone: controlToServerAddr.Zone,
	}

	// Data address is port+2:
	dataAddr := &net.UDPAddr{
		IP:   controlToServerAddr.IP,
		Port: controlToServerAddr.Port + 2,
		Zone: controlToServerAddr.Zone,
	}

	//netAddress := (*net.UDPAddr)(nil)
	//addrs, err := netInterface.Addrs()
	//if err == nil {
	//	fmt.Printf("Addresses for '%s':\n", netInterface.Name)
	//	for _, a := range addrs {
	//		fmt.Printf("  %s %s\n", a.Network(), a.String())
	//	}
	//}

	c := &Multicast{
		netInterface:        netInterface,
		datagramSize:        65000,
		bufferPacketCount:   10000,
		ttl:                 8,
		loopback:            false,
		controlToServerAddr: controlToServerAddr,
		controlToClientAddr: controlToClientAddr,
		dataAddr:            dataAddr,
	}
	return c, nil
}

func (m *Multicast) ListensControlToServer() error {
	controlToServerConn, err := net.ListenMulticastUDP("udp", m.netInterface, m.controlToServerAddr)
	if err != nil {
		return err
	}
	m.controlToServerConn = controlToServerConn
	m.setConnectionProperties(m.controlToServerConn)
	m.ControlToServer = make(chan UDPMessage)
	go m.receiveLoop(m.controlToServerConn, m.ControlToServer)
	return nil
}

func (m *Multicast) ListensControlToClient() error {
	controlToClientConn, err := net.ListenMulticastUDP("udp", m.netInterface, m.controlToClientAddr)
	if err != nil {
		return err
	}
	m.controlToClientConn = controlToClientConn
	m.setConnectionProperties(m.controlToClientConn)
	m.ControlToClient = make(chan UDPMessage)
	go m.receiveLoop(m.controlToClientConn, m.ControlToClient)
	return nil
}

func (m *Multicast) ListensData() error {
	dataConn, err := net.ListenMulticastUDP("udp", m.netInterface, m.dataAddr)
	if err != nil {
		return err
	}

	m.dataConn = dataConn
	m.setConnectionProperties(m.dataConn)
	m.Data = make(chan UDPMessage)
	go m.receiveLoop(m.dataConn, m.Data)
	return nil
}

func (m *Multicast) SendsControlToServer() error {
	controlToServerConn, err := net.ListenMulticastUDP("udp", m.netInterface, m.controlToServerAddr)
	if err != nil {
		return err
	}
	m.controlToServerConn = controlToServerConn
	m.setConnectionProperties(m.controlToServerConn)
	return nil
}

func (m *Multicast) SendsControlToClient() error {
	controlToClientConn, err := net.ListenMulticastUDP("udp", m.netInterface, m.controlToClientAddr)
	if err != nil {
		return err
	}
	m.controlToClientConn = controlToClientConn
	m.setConnectionProperties(m.controlToClientConn)
	return nil
}

func (m *Multicast) SendsData() error {
	dataConn, err := net.ListenMulticastUDP("udp", m.netInterface, m.dataAddr)
	if err != nil {
		return err
	}

	m.dataConn = dataConn
	m.setConnectionProperties(m.dataConn)
	return nil
}

func (m *Multicast) Close() error {
	if m.controlToServerConn != nil {
		err := m.controlToServerConn.Close()
		if err != nil {
			return err
		}
	}
	if m.controlToClientConn != nil {
		err := m.controlToClientConn.Close()
		if err != nil {
			return err
		}
	}
	if m.dataConn != nil {
		err := m.dataConn.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Multicast) setDatagramSize(c *net.UDPConn) error {
	err := c.SetReadBuffer(m.datagramSize * m.bufferPacketCount)
	if err != nil {
		return err
	}
	err = c.SetWriteBuffer(m.datagramSize * m.bufferPacketCount)
	if err != nil {
		return err
	}
	return nil
}

func (m *Multicast) setTTL(c *net.UDPConn) error {
	err := setSocketOptionInt(c, syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, m.ttl)
	if err != nil {
		return err
	}
	return nil
}

func (m *Multicast) setLoopback(c *net.UDPConn) error {
	lp := 0
	if m.loopback {
		lp = -1
	}
	err := setSocketOptionInt(c, syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, lp)
	if err != nil {
		return err
	}
	return nil
}

func (m *Multicast) setConnectionProperties(c *net.UDPConn) error {
	if err := m.setDatagramSize(c); err != nil {
		return err
	}
	if err := m.setTTL(c); err != nil {
		return err
	}
	if err := m.setLoopback(c); err != nil {
		return err
	}
	return nil
}

func (m *Multicast) SetDatagramSize(datagramSize int) {
	m.datagramSize = datagramSize
}

func (m *Multicast) SetTTL(ttl int) {
	m.ttl = ttl
}

func (m *Multicast) SetLoopback(enable bool) {
	m.loopback = enable
}

func (m *Multicast) MaxMessageSize() int {
	return m.datagramSize
}

func (m *Multicast) receiveLoop(conn *net.UDPConn, ch chan UDPMessage) error {
	// Start a message receive loop:
	for {
		buf := make([]byte, m.MaxMessageSize())
		n, recvAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			ch <- UDPMessage{Error: err}
			return err
		}
		ch <- UDPMessage{Data: buf[0:n], SourceAddress: recvAddr}
	}
	return nil
}

func (m *Multicast) SendControlToServer(msg []byte) (int, error) {
	n, err := m.controlToServerConn.WriteToUDP(msg, m.controlToServerAddr)
	return n, err
}

func (m *Multicast) SendControlToClient(msg []byte) (int, error) {
	n, err := m.controlToClientConn.WriteToUDP(msg, m.controlToClientAddr)
	return n, err
}

func (m *Multicast) SendData(msg []byte) (int, error) {
	n, err := m.dataConn.WriteToUDP(msg, m.dataAddr)
	return n, err
}
