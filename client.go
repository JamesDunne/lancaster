// client.go
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"time"
)

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

func NewClient(m *Multicast) *Client {
	return &Client{
		m:     m,
		state: ExpectAnnouncement,
	}
}

func (c *Client) Run() error {
	c.m.SendsControlToServer()
	c.m.ListensControlToClient()
	c.m.ListensData()

	logError := func(err error) {
		if err == nil {
			return
		}
		fmt.Fprintf(os.Stderr, "%s\n", err)
	}

	// Start by expecting an announcment message:
	c.state = ExpectAnnouncement
	c.hashId = nil

	// Start ticking every second to measure bandwidth:
	oneSecond := time.Tick(time.Second)
	c.lastTime = time.Now()
	c.startTime = c.lastTime
	c.lastBytesReceived = 0

	// Main message loop:
loop:
	for {
		err := error(nil)

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

		case <-oneSecond:
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
	fmt.Printf("%v elapsed\n", c.endTime.Sub(c.startTime))

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
	fmt.Printf("%15.2f B/s     %5.2f%% complete    \r", float64(byteCount)/sec, pct)

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
			// TODO: add some sort of subscribe feature for end users in case of multiple transfers
			c.hashId = hashId
			_ = data

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
			// Ignore message not for us:
			return nil
		}

		switch op {
		case RespondMetadataHeader:
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
			// Ignore message not for us:
			return nil
		}

		switch op {
		case RespondMetadataSection:
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
		buf := bytes.NewBuffer(make([]byte, 0, 8*2))
		binary.Write(buf, byteOrder, c.lastAck.start)
		binary.Write(buf, byteOrder, c.lastAck.endEx)
		_, err = c.m.SendControlToServer(controlToServerMessage(c.hashId, AckDataSection, buf.Bytes()))
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

	files := make([]TarballFile, 0, fileCount)
	for n := uint32(0); n < fileCount; n++ {
		f := TarballFile{}
		readString(&f.Path)
		readPrimitive(&f.Size)
		readPrimitive(&f.Mode)
		if err != nil {
			return err
		}

		files = append(files, f)
	}

	// Create a writer:
	c.tb, err = NewVirtualTarballWriter(files)
	if err != nil {
		return err
	}
	if c.tb.size != size {
		return errors.New("calculated tarball size does not match specified")
	}
	c.nakRegions = NewNakRegions(c.tb.size)

	fmt.Print("Receiving files:\n")
	for _, f := range c.tb.files {
		fmt.Printf("  %v %15d '%s'\n", f.Mode, f.Size, f.Path)
	}

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
