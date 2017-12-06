package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

type Server struct {
	m  *Multicast
	tb *VirtualTarballReader

	announceTicker <-chan time.Time
	announceMsg    []byte

	metadataHeader   []byte
	metadataSections [][]byte

	lastClientDataRequest time.Time
	enableDataSend        chan time.Time

	nakRegions  *NakRegions
	nextRegion  int64
	regionSize  uint16
	regionCount int64
}

func NewServer(m *Multicast, tb *VirtualTarballReader) *Server {
	return &Server{
		m:  m,
		tb: tb,
	}
}

func (s *Server) Run() error {
	err := (error)(nil)
	defer func() {
		err = s.m.Close()
	}()

	// Construct metadata sections:
	fmt.Print("Starting server...\n")
	if err = s.buildMetadata(); err != nil {
		return err
	}

	s.regionSize = uint16(s.m.MaxMessageSize() - (protocolDataMsgSize))
	s.nextRegion = 0
	s.regionCount = s.tb.size / int64(s.regionSize)
	if int64(s.regionSize)*s.regionCount < s.tb.size {
		s.regionCount++
	}
	fmt.Printf("region size %v, count %v\n", s.regionSize, s.regionCount)

	s.nakRegions = NewNakRegions(s.tb.size)

	// Let Multicast know what channels we're interested in sending/receiving:
	s.m.SendsControlToClient()
	s.m.SendsData()
	s.m.ListensControlToServer()

	// Tick to send a server announcement:
	s.announceTicker = time.Tick(1 * time.Second)

	// Create an announcement message:
	s.announceMsg = controlToClientMessage(s.tb.HashId(), AnnounceTarball, nil)

	// Channel to trigger enable data-sending mode:
	s.enableDataSend = make(chan time.Time, 1)

	// Send/recv loop:
	for {
		select {
		case ctrl := <-s.m.ControlToServer:
			if ctrl.Error != nil {
				return ctrl.Error
			}
			// Process client requests:
			s.processControl(ctrl)
		case <-s.announceTicker:
			// Announce transfer available:
			_, err := s.m.SendControlToClient(s.announceMsg)
			if err != nil {
				return err
			}
		case <-s.enableDataSend:
			// Switch to data-sending mode:
			if err = s.sendLoop(); err != nil {
				return err
			}
		}
	}

	return err
}

func (s *Server) sendLoop() error {
	err := error(nil)
	stopSendingTimer := time.After(2 * time.Second)

	for {
		select {
		case ctrl := <-s.m.ControlToServer:
			if ctrl.Error != nil {
				return ctrl.Error
			}
			// Process client requests:
			s.processControl(ctrl)
		case <-s.announceTicker:
			// Announce transfer available:
			_, err := s.m.SendControlToClient(s.announceMsg)
			if err != nil {
				return err
			}
		case <-s.enableDataSend:
			// Extend timer to keep sending data:
			stopSendingTimer = time.After(2 * time.Second)
		case <-stopSendingTimer:
			// Disable data sending mode and go back to announcement mode:
			return nil
		default:
			// Send next region chunk out:
			n := 0
			buf := make([]byte, s.regionSize)
			n, err = s.tb.ReadAt(buf, s.nextRegion)
			if err == ErrOutOfRange {
				fmt.Printf("ReadAt: %s\n", err)
				return nil
			}
			if err != nil {
				return err
			}
			buf = buf[:n]

			fmt.Printf("write: %v %v\n", s.nextRegion, len(buf))
			m := 0
			dataMsg := dataMessage(s.tb.HashId(), s.nextRegion, buf)
			m, err = s.m.SendData(dataMsg)
			if err != nil {
				return err
			}
			if m < len(dataMsg) {
				fmt.Printf("m<n: %v < %v\n", m, len(dataMsg))
			}

			// Advance to next region:
			s.nextRegion += int64(n)
			if s.nextRegion >= s.tb.size {
				s.nextRegion = 0
			}

			// Filter it out of NAKed regions:
			nextNak := s.nakRegions.NextNakRegion(s.nextRegion)
			if nextNak == -1 {
				return nil
			}
			s.nextRegion = nextNak
		}
	}

	return nil
}

func (s *Server) buildMetadata() error {
	err := error(nil)

	tb := s.tb
	mdSize := (2 + 8) + (len(tb.files) * (2 + 40 + 8 + 4 + 32))
	mdBuf := bytes.NewBuffer(make([]byte, 0, mdSize))

	writePrimitive := func(data interface{}) {
		if err == nil {
			err = binary.Write(mdBuf, byteOrder, data)
		}
	}
	writeString := func(s string) {
		writePrimitive(uint16(len(s)))
		if err == nil {
			_, err = mdBuf.WriteString(s)
		}
	}
	writeBytes := func(b []byte) {
		if err == nil {
			_, err = mdBuf.Write(b)
		}
	}

	writePrimitive(tb.size)
	writePrimitive(uint32(len(tb.files)))
	for _, f := range tb.files {
		writeString(f.Path)
		writePrimitive(f.Size)
		writePrimitive(f.Mode)
		writeBytes(f.Hash)
	}
	if err != nil {
		return err
	}

	md := mdBuf.Bytes()

	sectionSize := (s.m.MaxMessageSize() - (protocolControlPrefixSize + metadataSectionMsgSize))
	sectionCount := len(md) / sectionSize
	if sectionCount*sectionSize < len(md) {
		sectionCount++
	}

	// Slice into sections:
	s.metadataSections = make([][]byte, 0, sectionCount)
	o := 0
	for n := 0; n < sectionCount; n++ {
		// Determine end point of metadata slice:
		l := sectionSize
		if o+l > len(md) {
			l = len(md) - o
		}

		// Prepend section with uint16 of `n`:
		ms := make([]byte, metadataSectionMsgSize, metadataSectionMsgSize+l)
		byteOrder.PutUint16(ms[0:2], uint16(n))
		ms = append(ms, md[o:o+l]...)

		// Add section to list:
		s.metadataSections = append(s.metadataSections, ms)
		o += l
	}

	// Create metadata header to describe how many sections there are:
	s.metadataHeader = make([]byte, metadataHeaderMsgSize)
	byteOrder.PutUint16(s.metadataHeader, uint16(sectionCount))

	return nil
}

func (s *Server) processControl(ctrl UDPMessage) error {
	//fmt.Printf("ctrlrecv\n%s", hex.Dump(ctrl.Data))
	hashId, op, data, err := extractServerMessage(ctrl)
	if err != nil {
		return err
	}

	if bytes.Compare(hashId, s.tb.HashId()) != 0 {
		// Ignore message not for us:
		return nil
	}

	switch op {
	case RequestMetadataHeader:
		_ = data

		// Respond with metadata header:
		s.m.SendControlToClient(controlToClientMessage(hashId, RespondMetadataHeader, s.metadataHeader))
	case RequestMetadataSection:
		sectionIndex := byteOrder.Uint16(data[0:2])
		if sectionIndex >= uint16(len(s.metadataSections)) {
			// Out of range
			return nil
		}

		// Send metadata section message:
		section := s.metadataSections[sectionIndex]
		s.m.SendControlToClient(controlToClientMessage(hashId, RespondMetadataSection, section))
	case AckDataSection:
		// Read ACK and record it:
		ack := Region{
			start: int64(byteOrder.Uint64(data[0:8])),
			endEx: int64(byteOrder.Uint64(data[8:16])),
		}
		s.nakRegions.Ack(ack.start, ack.endEx)
		// Keep data-send mode alive:
		s.enableDataSend <- time.Now()
	}

	return nil
}
