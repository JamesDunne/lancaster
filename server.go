package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)
import "github.com/dustin/go-humanize"
import "golang.org/x/time/rate"

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
	limiter                 *rate.Limiter

	nextLock    sync.Mutex
	nakRegions  *NakRegions
	nextRegion  int64
	regionSize  uint16
	regionCount int64

	bytesSent     int64
	bytesSentLast int64
	timeLast      time.Time
	lastRate      float64
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
		limiter:   rate.NewLimiter(rate.Limit((1024*1024*1024)/(m.datagramSize*8)), 20),
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

	// Initialize with fully ACKed so that resuming clients send NAK state:
	s.nakRegions = NewNakRegions(s.tb.size)
	//s.nakRegions.Ack(0, s.tb.size)

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
			if isENOBUFS(err) {
				time.Sleep(bufferFullTimeoutMilli * time.Millisecond)
				err = nil
			}

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
	rightMeow := time.Now()
	sec := rightMeow.Sub(s.timeLast).Seconds()
	{
		byteCount := s.bytesSent - s.bytesSentLast
		s.lastRate = float64(byteCount) / sec
		s.bytesSentLast = s.bytesSent
		s.timeLast = rightMeow
	}

	fmt.Printf("\b%9s/s        [%s]\r", humanize.IBytes(uint64(s.lastRate)), s.nakRegions.ASCIIMeterPosition(48, s.nextRegion))
}

// goroutine to only send data while clients request it:
func (s *Server) sendDataLoop() {
	for {
		// Wait until we're requested by at least 1 client to send data:
		<-s.allowSend

		// Each client ACK buys some time of data sending:
		timer := time.After(resendTimeout)
	sendloop:
		for {
			select {
			case <-timer:
				break sendloop
			default:
			}

			// Rate limit our sending:
			s.limiter.Wait(context.Background())

			// Send next data region:
			err := s.sendData()
			if isENOBUFS(err) {
				fmt.Print("\r!")
				s.limiter.SetLimit(s.limiter.Limit() * 0.85)
				err = nil
			} else {
				s.limiter.SetLimit(s.limiter.Limit() * 1.025)
			}

			if err != nil {
				fmt.Printf("%s\n", err)
			}
		}
	}
}

func (s *Server) sendData() error {
	err := error(nil)

	// Lock access so NAKs are consistent:
	s.nextLock.Lock()
	defer s.nextLock.Unlock()

	lastRegion := s.nextRegion

	// Filter out ACKed regions:
	//fmt.Printf("\r\bold = %15d\n", s.nextRegion)
	nextNak := s.nakRegions.NextNakRegion(s.nextRegion)
	if nextNak != -1 {
		//fmt.Printf("\bnew = %15d\n", nextNak)
		s.nextRegion = nextNak
	}

	// Read data from virtual tarball:
	n := 0
	buf := make([]byte, s.regionSize)
	n, err = s.tb.ReadAt(buf, s.nextRegion)
	if err == ErrOutOfRange {
		fmt.Printf("ReadAt: %s\n", err)
		return nil
	}
	if err != nil {
		// Rewind due to error:
		s.nextRegion = lastRegion
		return err
	}
	buf = buf[:n]

	// Send data message:
	m := 0
	dataMsg := dataMessage(s.hashId, s.nextRegion, buf)
	m, err = s.m.SendData(dataMsg)
	if err != nil {
		// Rewind due to error:
		s.nextRegion = lastRegion
		return err
	}
	if m < len(buf) {
		fmt.Printf("m < buf: %d < %d\n", m, len(buf))
	}

	// ACK last send region:
	s.nakRegions.Ack(s.nextRegion, s.nextRegion+int64(n))
	s.bytesSent += int64(n)

	// Advance to next region:
	s.nextRegion += int64(n)
	if s.nextRegion >= s.tb.size {
		s.nextRegion = 0
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
		s.nextLock.Lock()
		i := 0
		//var ack Region
		//ack, i = readRegion(data, i)
		//s.nakRegions.Ack(ack.start, ack.endEx)
		for i < len(data) {
			var nak Region
			nak, i = readRegion(data, i)
			//fmt.Printf("\bnak [%15v %15v]\n", nak.start, nak.endEx)
			s.nakRegions.Nak(nak.start, nak.endEx)
		}
		s.nextLock.Unlock()

		// Allow sending data with a non-blocking channel send:
		s.lastClientDataRequest = time.Now()
		select {
		case s.allowSend <- empty{}:
		default:
		}
		return nil
	}

	if isENOBUFS(err) {
		fmt.Print("\r!")
		time.Sleep(bufferFullTimeoutMilli * time.Millisecond)
		err = nil
	}

	return err
}

func readRegion(data []byte, i int) (Region, int) {
	start, n := binary.Uvarint(data[i:])
	i += n
	endEx, n := binary.Uvarint(data[i:])
	i += n
	return Region{int64(start), int64(endEx)}, i
}
