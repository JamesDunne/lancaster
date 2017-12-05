// client.go
package main

import (
	"encoding/hex"
	"fmt"
	"os"
)

type Client struct {
	m  *Multicast
	tb *VirtualTarballReader
}

func NewClient(m *Multicast) *Client {
	return &Client{
		m,
		nil,
	}
}

func (c *Client) Run() error {
	c.m.SendsControlToServer()
	c.m.ListensControlToClient()
	c.m.ListensData()

	// Read UDP messages from multicast:
	for {
		select {
		case ctrl := <-c.m.ControlToClient:
			if ctrl.Error != nil {
				return ctrl.Error
			}
			fmt.Printf("ctrlrecv\n%s", hex.Dump(ctrl.Data))

			err := c.processControl(ctrl)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
			}
		case msg := <-c.m.Data:
			if msg.Error != nil {
				return msg.Error
			}
			fmt.Printf("datarecv\n%s", hex.Dump(msg.Data))
		}
	}

	return c.m.Close()
}

func (c *Client) processControl(ctrl UDPMessage) error {
	hashId, op, data, err := extractClientMessage(ctrl)
	if err != nil {
		return err
	}

	switch op {
	case AnnounceTarball:
		fmt.Printf("announcement\n")
		_ = data
		_ = hashId

		// Request metadata:
		//c.m.SendControlToServer()
	}

	return nil
}
