// client.go
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"
)
import "github.com/dustin/go-humanize"

type ClientState int

const (
	ExpectAnnouncement = ClientState(iota)
	ExpectMetadataHeader
	ExpectMetadataSections
	ExpectDataSections
	Done
)

const resendTimeout = 500 * time.Millisecond

type Client struct {
	m  *Multicast
	tb *VirtualTarballWriter

	options ClientOptions

	state       ClientState
	resendTimer <-chan time.Time

	hashId               []byte
	metadataSectionCount uint16
	metadataSections     [][]byte
	nextSectionIndex     uint16

	nakRegions *NakRegions
	lastAck    Region

	bytesReceived     int64
	lastBytesReceived int64
	lastTime          time.Time

	startTime time.Time
	endTime   time.Time
}

type ClientOptions struct {
	TarballOptions VirtualTarballOptions
	HashId         []byte
	StorePath      string
	RefreshRate    time.Duration
}

func NewClient(m *Multicast, options ClientOptions) *Client {
	if options.RefreshRate <= time.Duration(0) {
		options.RefreshRate = time.Second
	}

	return &Client{
		m:       m,
		options: options,
		state:   ExpectAnnouncement,
		hashId:  options.HashId,
	}
}

func (c *Client) Run() error {
	err := error(nil)

	err = c.m.SendsControlToServer()
	if err != nil {
		return err
	}
	err = c.m.ListensControlToClient()
	if err != nil {
		return err
	}
	err = c.m.ListensData()
	if err != nil {
		return err
	}

	logError := func(err error) {
		if err == nil {
			return
		}
		fmt.Fprintf(os.Stderr, "%s\n", err)
	}

	// Start by expecting an announcment message:
	c.state = ExpectAnnouncement

	// Start ticking every second to measure bandwidth:
	refreshTimer := time.Tick(c.options.RefreshRate)
	c.lastTime = time.Now()
	c.startTime = c.lastTime
	c.lastBytesReceived = 0

	// Main message loop:
loop:
	for {
		select {
		case msg := <-c.m.ControlToClient:
			if msg.Error != nil {
				return msg.Error
			}

			err = c.processControl(msg)
			logError(err)
			if c.state == Done {
				break loop
			}

		case msg := <-c.m.Data:
			if msg.Error != nil {
				return msg.Error
			}

			err = c.processData(msg)
			logError(err)
			if c.state == Done {
				break loop
			}

		case <-c.resendTimer:
			// Resend a request that might have gotten lost:
			err = c.ask()
			logError(err)
			if c.state == Done {
				break loop
			}

		case <-refreshTimer:
			// Measure and report receive-bandwidth:
			c.reportBandwidth()

			if c.state == Done {
				break loop
			}
		}
	}

	// Final report:
	c.reportBandwidth()
	fmt.Println()

	// Elapsed time:
	c.endTime = time.Now()
	diff := c.endTime.Sub(c.startTime)
	fmt.Printf("%v elapsed %15s/s avg\n", diff, humanize.IBytes(uint64(float64(c.bytesReceived)/diff.Seconds())))

	// Close virtual tarball writer:
	if c.tb != nil {
		if err := c.tb.Close(); err != nil {
			return err
		}
	}

	// Close multicast sockets:
	return c.m.Close()
}

func (c *Client) reportBandwidth() {
	byteCount := c.bytesReceived - c.lastBytesReceived
	rightMeow := time.Now()
	sec := rightMeow.Sub(c.lastTime).Seconds()

	pct := float64(0.0)
	if c.nakRegions != nil {
		pct = float64(c.bytesReceived) * 100.0 / float64(c.nakRegions.size)
	}
	nakMeter := ""
	if c.nakRegions != nil {
		nakMeter = c.nakRegions.ASCIIMeter(48)
	}
	fmt.Printf("%s/s %6.2f%% [%s]\r", humanize.IBytes(uint64(float64(byteCount)/sec)), pct, nakMeter)

	c.lastBytesReceived = c.bytesReceived
	c.lastTime = rightMeow
}

func (c *Client) processControl(msg UDPMessage) error {
	hashId, op, data, err := extractClientMessage(msg)
	if err != nil {
		return err
	}

	switch c.state {
	case ExpectAnnouncement:
		switch op {
		case AnnounceTarball:
			//fmt.Printf("announce %s\n", hex.EncodeToString(hashId))
			if c.hashId == nil {
				// If client has not specified a hashId to listen for, accept the first one that's announced:
				c.hashId = hashId
			} else if compareHashes(c.hashId, hashId) != 0 {
				// These are not the droids we're looking for.
				//fmt.Printf("\rIgnore announcement for %s; only interested in %s\n", hex.EncodeToString(hashId), hex.EncodeToString(c.hashId))
				return nil
			}

			// Request metadata header:
			c.state = ExpectMetadataHeader
			if err = c.ask(); err != nil {
				return err
			}
		default:
			// ignore
		}

	case ExpectMetadataHeader:
		if compareHashes(c.hashId, hashId) != 0 {
			// These are not the droids we're looking for.
			//fmt.Printf("Ignore announcement for %s; only interested in %s\n", hex.EncodeToString(hashId), hex.EncodeToString(c.hashId))
			return nil
		}

		switch op {
		case RespondMetadataHeader:
			//fmt.Printf("metaheader %s\n", hex.EncodeToString(hashId))
			// Read count of sections:
			c.metadataSectionCount = byteOrder.Uint16(data[0:2])
			c.metadataSections = make([][]byte, c.metadataSectionCount)

			// Request metadata sections:
			c.state = ExpectMetadataSections
			c.nextSectionIndex = 0
			if err = c.ask(); err != nil {
				return err
			}
		default:
			// ignore
		}

	case ExpectMetadataSections:
		if compareHashes(c.hashId, hashId) != 0 {
			// These are not the droids we're looking for.
			//fmt.Printf("Ignore announcement for %s; only interested in %s\n", hex.EncodeToString(hashId), hex.EncodeToString(c.hashId))
			return nil
		}

		switch op {
		case RespondMetadataSection:
			//fmt.Printf("metasection %s\n", hex.EncodeToString(hashId))

			sectionIndex := byteOrder.Uint16(data[0:2])
			if sectionIndex == c.nextSectionIndex {
				c.metadataSections[sectionIndex] = make([]byte, len(data[2:]))
				copy(c.metadataSections[sectionIndex], data[2:])

				c.nextSectionIndex++
				if c.nextSectionIndex >= c.metadataSectionCount {
					// Done receiving all metadata sections; decode:
					if err = c.decodeMetadata(); err != nil {
						return err
					}

					// Start expecting data sections:
					c.state = ExpectDataSections
					if err = c.ask(); err != nil {
						return err
					}
					return nil
				}
			}

			// Request next metadata sections:
			c.state = ExpectMetadataSections
			if err = c.ask(); err != nil {
				return err
			}
		default:
			// ignore
		}

	case ExpectDataSections:
		// Not interested in control messages really at this time. Maybe introduce server death messages?
	}

	return nil
}

func (c *Client) ask() error {
	err := (error)(nil)

	switch c.state {
	case ExpectMetadataHeader:
		_, err = c.m.SendControlToServer(controlToServerMessage(c.hashId, RequestMetadataHeader, nil))
		if err != nil {
			return err
		}
	case ExpectMetadataSections:
		// Request next metadata section:
		req := make([]byte, 2)
		byteOrder.PutUint16(req[0:2], uint16(c.nextSectionIndex))
		_, err = c.m.SendControlToServer(controlToServerMessage(c.hashId, RequestMetadataSection, req))
		if err != nil {
			return err
		}
	case ExpectDataSections:
		// Send the last ACKed region to get a new region:
		//fmt.Printf("ack: [%v %v]\n", c.lastAck.start, c.lastAck.endEx)
		max := c.m.MaxMessageSize() - (2*8 + protocolControlPrefixSize)
		bytes := make([]byte, max)
		byteOrder.PutUint64(bytes[0:8], uint64(c.lastAck.start))
		byteOrder.PutUint64(bytes[8:16], uint64(c.lastAck.endEx))
		// Send as many NAKed regions as we can fit in a message so the server doesnt waste time sending already-ACKed sections:
		i := 16
		for _, nak := range c.nakRegions.Naks() {
			if i >= max-2*binary.MaxVarintLen64 {
				break
			}
			// Skip NAKed regions until last ACKed region:
			if nak.endEx < c.lastAck.endEx {
				continue
			}
			i += binary.PutUvarint(bytes[i:], uint64(nak.start))
			i += binary.PutUvarint(bytes[i:], uint64(nak.endEx))
		}
		// Loop back around and add any NAKs before last ACK:
		for _, nak := range c.nakRegions.Naks() {
			if i >= max-2*binary.MaxVarintLen64 {
				break
			}
			// Skip NAKed regions after last ACKed region:
			if nak.endEx >= c.lastAck.endEx {
				break
			}
			i += binary.PutUvarint(bytes[i:], uint64(nak.start))
			i += binary.PutUvarint(bytes[i:], uint64(nak.endEx))
		}
		//fmt.Printf("%s", hex.Dump(bytes[:i]))
		_, err = c.m.SendControlToServer(controlToServerMessage(c.hashId, AckDataSection, bytes[:i]))
		if err != nil {
			return err
		}
	case Done:
	default:
		return nil
	}

	// Start a timer for next ask in case this one got lost:
	c.resendTimer = time.After(resendTimeout)
	return nil
}

func (c *Client) decodeMetadata() error {
	// Decode all metadata sections and create a VirtualTarballWriter to download against:
	md := bytes.Join(c.metadataSections, nil)
	mdBuf := bytes.NewBuffer(md)

	err := error(nil)
	readPrimitive := func(data interface{}) {
		if err == nil {
			err = binary.Read(mdBuf, byteOrder, data)
		}
	}
	readString := func(s *string) {
		if err != nil {
			return
		}

		strlen := uint16(0)
		readPrimitive(&strlen)
		if err != nil {
			return
		}

		strbuf := make([]byte, strlen)
		n := 0
		n, err = mdBuf.Read(strbuf)
		if err != nil {
			return
		}

		if uint16(n) != strlen {
			err = errors.New("unable to read string from message")
			return
		}

		*s = string(strbuf)
	}

	// Deserialize tarball metadata:
	size := int64(0)
	readPrimitive(&size)
	fileCount := uint32(0)
	readPrimitive(&fileCount)
	if err != nil {
		return err
	}

	files := make([]*TarballFile, 0, fileCount)
	for n := uint32(0); n < fileCount; n++ {
		f := &TarballFile{}
		readString(&f.Path)
		readPrimitive(&f.Size)
		readPrimitive(&f.Mode)
		readString(&f.SymlinkDestination)
		if err != nil {
			return err
		}

		files = append(files, f)
	}

	// Create a writer:
	c.tb, err = NewVirtualTarballWriter(files, c.options.TarballOptions)
	if err != nil {
		return err
	}
	if c.tb.size != size {
		return errors.New("calculated tarball size does not match specified")
	}
	c.nakRegions = NewNakRegions(c.tb.size)

	fmt.Print("Receiving files:\n")
	for _, f := range c.tb.files {
		fmt.Printf("  %v %15s '%s'\n", f.Mode, humanize.Comma(f.Size), f.Path)
	}

	fmt.Printf("%15s  ID: %s\n", humanize.Comma(c.tb.size), hex.EncodeToString(c.hashId))

	// Start elapsed timer:
	c.startTime = time.Now()

	return nil
}

func (c *Client) processData(msg UDPMessage) error {
	// Not ready for data yet:
	if c.tb == nil {
		//fmt.Print("not ready for data\n")
		return nil
	}

	// Decode data message:
	hashId, region, data, err := extractDataMessage(msg)
	if err != nil {
		return err
	}

	if compareHashes(c.hashId, hashId) != 0 {
		// Ignore message not for us:
		//fmt.Print("data msg ignored\n")
		return nil
	}

	c.lastAck = Region{start: region, endEx: region + int64(len(data))}

	if c.nakRegions.IsAcked(c.lastAck.start, c.lastAck.endEx) {
		// Already ACKed:
		allDone := c.nakRegions.IsAllAcked()
		if allDone {
			c.state = Done
		}

		return c.ask()
	}

	// ACK the region:
	err = c.nakRegions.Ack(c.lastAck.start, c.lastAck.endEx)
	if err != nil {
		return err
	}
	// Write the data:
	n := 0
	n, err = c.tb.WriteAt(data, region)
	if err != nil {
		return err
	}
	_ = n

	c.bytesReceived += int64(len(data))

	allDone := c.nakRegions.IsAllAcked()
	if allDone {
		c.state = Done
	}

	// Ask for more data:
	return c.ask()
}
