// client.go
package main

import (
	"encoding/hex"
	"fmt"
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

	ack := []byte("ack")

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
			if len(ctrl.Data) >= 1 {
				switch ControlToClientOp(ctrl.Data[0]) {
				case AnnounceTarball:
					hashId := ctrl.Data[1:]
					fmt.Printf("announcement: %v\n", hashId)
				}
			}
		case msg := <-c.m.Data:
			if msg.Error != nil {
				return msg.Error
			}
			fmt.Printf("datarecv\n%s", hex.Dump(msg.Data))

			_, err := c.m.SendControlToServer(ack)
			if err != nil {
				return err
			}
			fmt.Printf("ctrlsent\n%s", hex.Dump(ack))
		}
	}

	return c.m.Close()
}
