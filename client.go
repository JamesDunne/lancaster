// client.go
package main

import (
	"bytes"
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

	nakRregions NakRegions
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

	// Main message loop:
	for {
		err := error(nil)

		select {
		case msg := <-c.m.ControlToClient:
			if msg.Error != nil {
				return msg.Error
			}

			err = c.processControl(msg)
			logError(err)

		case msg := <-c.m.Data:
			if msg.Error != nil {
				return msg.Error
			}
			err = c.processData(msg)
			logError(err)

		case <-c.resendTimer:
			// Resend a request that might have gotten lost:
			err = c.ask()
			logError(err)
		}
	}

	return c.m.Close()
}

func (c *Client) processControl(msg UDPMessage) error {
	hashId, op, data, err := extractClientMessage(msg)
	if err != nil {
		return err
	}

	//fmt.Printf("ctrlrecv\n%s", hex.Dump(msg.Data))

	switch c.state {
	case ExpectAnnouncement:
		switch op {
		case AnnounceTarball:
			fmt.Printf("announcement\n")
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
		if bytes.Compare(c.hashId, hashId) != 0 {
			// Ignore message not for us:
			return nil
		}

		switch op {
		case RespondMetadataHeader:
			fmt.Printf("metadata header\n")
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
		if bytes.Compare(c.hashId, hashId) != 0 {
			// Ignore message not for us:
			return nil
		}

		switch op {
		case RespondMetadataSection:
			fmt.Printf("metadata section\n")
			sectionIndex := byteOrder.Uint16(data[0:2])
			if sectionIndex == c.nextSectionIndex {
				c.metadataSections[sectionIndex] = data[2:]

				c.nextSectionIndex++
				if c.nextSectionIndex >= c.metadataSectionCount {
					// Done.
					// TODO: decode metadata and create VirtualTarballWriter
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
	}

	return nil
}

func (c *Client) ask() error {
	err := (error)(nil)
	//fmt.Printf("datarecv\n%s", hex.Dump(msg.Data))

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
	default:
		return nil
	}

	// Start a timer for next ask in case this one got lost:
	c.resendTimer = time.After(resendTimeout)
	return nil
}

func (c *Client) processData(msg UDPMessage) error {
	return nil
}
