// udp
package main

import (
	"net"
	"runtime"
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
	netInterface     *net.Interface
	datagramSize     int
	sendControlCount int
	recvControlCount int
	sendDataCount    int
	recvDataCount    int
	ttl              int
	loopback         bool

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

func NewMulticast(controlToServerAddr *net.UDPAddr, netInterface *net.Interface) (*Multicast, error) {
	// Control to-server address is port+0:
	if controlToServerAddr.Port == 0 {
		// Set default port if not specified:
		controlToServerAddr.Port = 1360
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
		sendControlCount:    2,
		recvControlCount:    32,
		sendDataCount:       64,
		recvDataCount:       64,
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

	if err := m.setConnectionProperties(m.controlToServerConn); err != nil {
		return err
	}
	if err := m.controlToServerConn.SetReadBuffer(m.datagramSize * m.recvControlCount); err != nil {
		return err
	}
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
	if err := m.setConnectionProperties(m.controlToClientConn); err != nil {
		return err
	}
	if err := m.controlToClientConn.SetReadBuffer(m.datagramSize * m.recvControlCount); err != nil {
		return err
	}
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
	if err := m.setConnectionProperties(m.dataConn); err != nil {
		return err
	}
	if err := m.dataConn.SetReadBuffer(m.datagramSize * m.recvDataCount); err != nil {
		return err
	}
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

	if err := m.setConnectionProperties(m.controlToServerConn); err != nil {
		return err
	}
	if err := m.controlToServerConn.SetWriteBuffer(m.datagramSize * m.sendControlCount); err != nil {
		return err
	}

	return nil
}

func (m *Multicast) SendsControlToClient() error {
	controlToClientConn, err := net.ListenMulticastUDP("udp", m.netInterface, m.controlToClientAddr)
	if err != nil {
		return err
	}
	m.controlToClientConn = controlToClientConn

	if err := m.setConnectionProperties(m.controlToClientConn); err != nil {
		return err
	}
	if err := m.controlToClientConn.SetWriteBuffer(m.datagramSize * m.sendControlCount); err != nil {
		return err
	}

	return nil
}

func (m *Multicast) SendsData() error {
	dataConn, err := net.ListenMulticastUDP("udp", m.netInterface, m.dataAddr)
	if err != nil {
		return err
	}

	m.dataConn = dataConn
	if err := m.setConnectionProperties(m.dataConn); err != nil {
		return err
	}
	if err := m.dataConn.SetWriteBuffer(m.datagramSize * m.sendDataCount); err != nil {
		return err
	}

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
	// Lock receive loops to specific CPU core:
	runtime.LockOSThread()

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
