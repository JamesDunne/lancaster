package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)
import "github.com/dustin/go-humanize"

type empty struct{}

type Server struct {
	m  *Multicast
	tb *VirtualTarballReader

	options ServerOptions

	hashId []byte

	announceTicker <-chan time.Time
	announceMsg    []byte

	metadataHeader   []byte
	metadataSections [][]byte

	lastClientDataRequest   time.Time
	packetsSentSinceLastAck int
	allowSend               chan empty

	nakRegions  *NakRegions
	nextRegion  int64
	regionSize  uint16
	regionCount int64

	bytesSent     int64
	bytesSentLast int64
	timeLast      time.Time
}

type ServerOptions struct {
	RefreshRate time.Duration
}

func NewServer(m *Multicast, tb *VirtualTarballReader, options ServerOptions) *Server {
	if options.RefreshRate <= time.Duration(0) {
		options.RefreshRate = time.Second
	}

	return &Server{
		m:         m,
		tb:        tb,
		options:   options,
		hashId:    tb.HashId(),
		allowSend: make(chan empty, 1),
	}
}

func (s *Server) Run() error {
	err := (error)(nil)
	defer func() {
		err = s.m.Close()
	}()

	// Construct metadata sections:
	if err = s.buildMetadata(); err != nil {
		return err
	}

	s.regionSize = uint16(s.m.MaxMessageSize() - (protocolDataMsgPrefixSize))
	s.nextRegion = 0
	s.regionCount = s.tb.size / int64(s.regionSize)
	if int64(s.regionSize)*s.regionCount < s.tb.size {
		s.regionCount++
	}

	s.nakRegions = NewNakRegions(s.tb.size)

	// Let Multicast know what channels we're interested in sending/receiving:
	err = s.m.SendsControlToClient()
	if err != nil {
		return err
	}
	err = s.m.SendsData()
	if err != nil {
		return err
	}
	err = s.m.ListensControlToServer()
	if err != nil {
		return err
	}

	// Tick to send a server announcement:
	s.announceTicker = time.Tick(1 * time.Second)

	// Create an announcement message:
	s.announceMsg = controlToClientMessage(s.hashId, AnnounceTarball, nil)

	// Create a one-second ticker for reporting:
	refreshTimer := time.Tick(s.options.RefreshRate)

	fmt.Print("Started server\n")
	fmt.Printf("%15s  ID: %s\n", humanize.Comma(s.tb.size), hex.EncodeToString(s.hashId))

	// Send/recv loop:
	go s.sendDataLoop()

	for {
		select {
		case ctrl := <-s.m.ControlToServer:
			if ctrl.Error != nil {
				return ctrl.Error
			}
			// Process client requests:
			err := s.processControl(ctrl)
			if err != nil {
				fmt.Printf("%s\n", err)
			}
		case <-s.announceTicker:
			// Announce transfer available:
			//fmt.Printf("announce %s\n", hex.EncodeToString(s.hashId))

			_, err := s.m.SendControlToClient(s.announceMsg)
			if err != nil {
				fmt.Printf("%s\n", err)
			}
		case <-refreshTimer:
			s.reportBandwidth()
		}
	}

	fmt.Print("Stopped server\n")
	return err
}

func (s *Server) reportBandwidth() {
	byteCount := s.bytesSent - s.bytesSentLast
	rightMeow := time.Now()
	sec := rightMeow.Sub(s.timeLast).Seconds()

	fmt.Printf("%15s/s        [%s]\r", humanize.IBytes(uint64(float64(byteCount)/sec)), s.nakRegions.ASCIIMeterPosition(48, s.nextRegion))

	s.bytesSentLast = s.bytesSent
	s.timeLast = rightMeow
}

// goroutine to only send data while clients request it:
func (s *Server) sendDataLoop() {
	for {
		// Wait until we're requested by at least 1 client to send data:
		<-s.allowSend
		err := s.sendData()
		if err != nil {
			fmt.Printf("%s\n", err)
		}
	}
}

func (s *Server) sendData() error {
	err := error(nil)

	// Read data from virtual tarball:
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

	// Send data message:
	m := 0
	dataMsg := dataMessage(s.hashId, s.nextRegion, buf)
	m, err = s.m.SendData(dataMsg)
	if err != nil {
		return err
	}
	if m < len(buf) {
		fmt.Printf("m < buf: %d < %d\n", m, len(buf))
	}

	s.bytesSent += int64(m)

	// Advance to next region:
	s.nextRegion += int64(n)
	if s.nextRegion >= s.tb.size {
		s.nextRegion = 0
	}

	// Filter out ACKed regions:
	nextNak := s.nakRegions.NextNakRegion(s.nextRegion)
	if nextNak == -1 {
		return nil
	}
	s.nextRegion = nextNak

	// Keep sending new packets while clients are connected:
	//	if time.Now().Sub(s.lastClientDataRequest) <= 20*time.Millisecond {
	//		select {
	//		case s.allowSend <- empty{}:
	//		default:
	//		}
	//	}

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

	writePrimitive(tb.size)
	writePrimitive(uint32(len(tb.files)))
	fmt.Print("Files:\n")
	for _, f := range tb.files {
		writeString(f.Path)
		writePrimitive(f.Size)
		writePrimitive(f.Mode)
		writeString(f.SymlinkDestination)
		fmt.Printf("  %v %15s '%s'\n", f.Mode, humanize.Comma(f.Size), f.Path)
	}
	if err != nil {
		return err
	}

	// Slice into sections:
	md := mdBuf.Bytes()

	sectionSize := (s.m.MaxMessageSize() - (protocolControlPrefixSize + metadataSectionMsgSize))
	sectionCount := len(md) / sectionSize
	if sectionCount*sectionSize < len(md) {
		sectionCount++
	}

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
	hashId, op, data, err := extractServerMessage(ctrl)
	if err != nil {
		return err
	}

	if compareHashes(hashId, s.hashId) != 0 {
		// Ignore message not for us:
		//fmt.Printf("ignore message for %s; expecting for %s\n", hex.EncodeToString(hashId), hex.EncodeToString(s.hashId))
		return nil
	}

	switch op {
	case RequestMetadataHeader:
		_ = data

		// Respond with metadata header:
		_, err = s.m.SendControlToClient(controlToClientMessage(hashId, RespondMetadataHeader, s.metadataHeader))
	case RequestMetadataSection:
		sectionIndex := byteOrder.Uint16(data[0:2])
		if sectionIndex >= uint16(len(s.metadataSections)) {
			// Out of range
			return nil
		}

		// Send metadata section message:
		section := s.metadataSections[sectionIndex]
		_, err = s.m.SendControlToClient(controlToClientMessage(hashId, RespondMetadataSection, section))
	case AckDataSection:
		// Read ACK and record it:
		ack := Region{
			start: int64(byteOrder.Uint64(data[0:8])),
			endEx: int64(byteOrder.Uint64(data[8:16])),
		}
		if ack.start == 0 && ack.endEx == 0 {
			// New client means clear all ACKs:
			s.nakRegions.Clear()
		} else {
			// ACK region:
			s.nakRegions.Ack(ack.start, ack.endEx)
		}

		// Record known NAKs:
		i := 16
		for i < len(data) {
			v1, n := binary.Uvarint(data[i:])
			i += n
			v2, n := binary.Uvarint(data[i:])
			i += n
			s.nakRegions.Nak(int64(v1), int64(v2))
		}

		// Allow sending data with a non-blocking channel send:
		s.lastClientDataRequest = time.Now()
		select {
		case s.allowSend <- empty{}:
		default:
		}
		return nil
	}

	return err
}
