// client.go
package main

import (
	"encoding/hex"
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

	hashId      []byte
	nakRregions nakRegions
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
			fmt.Printf("ctrlrecv\n%s", hex.Dump(msg.Data))

			err = c.processControl(msg)
			logError(err)

		case msg := <-c.m.Data:
			if msg.Error != nil {
				return msg.Error
			}
			fmt.Printf("datarecv\n%s", hex.Dump(msg.Data))

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

	switch c.state {
	case ExpectAnnouncement:
		switch op {
		case AnnounceTarball:
			fmt.Printf("announcement\n")
			c.hashId = hashId
			_ = data

			// Request metadata:
			c.state = ExpectMetadataHeader
			if err = c.ask(); err != nil {
				return err
			}
		default:
			// ignore
		}

	case ExpectMetadataHeader:
		switch op {
		case RespondMetadataHeader:
			fmt.Printf("announcement\n")
			c.hashId = hashId
			_ = data

			// Request metadata:
			c.state = ExpectMetadataHeader
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

	switch c.state {
	case ExpectMetadataHeader:
		_, err = c.m.SendControlToServer(controlToServerMessage(c.hashId, RequestMetadataHeader, nil))
		if err != nil {
			return err
		}
	}

	// Start a timer for next ask in case this one got lost:
	c.resendTimer = time.After(resendTimeout)
	return nil
}

func (c *Client) processData(msg UDPMessage) error {
	return nil
}
