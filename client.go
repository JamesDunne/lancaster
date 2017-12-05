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

	go c.m.DataReceiveLoop()
	go c.m.ControlReceiveLoop()

	// Read UDP messages from multicast:
	for {
		select {
		case ctrl := <-c.m.Control:
			if ctrl.Error != nil {
				return ctrl.Error
			}
			fmt.Printf("ctrlrecv\n%s", hex.Dump(ctrl.Data))
			if len(ctrl.Data) >= 1 {
				switch ctrl.Data[0] {
				case 0x01:
					hashId := ctrl.Data[1:]
					fmt.Printf("announcement: %v\n", hashId)
				}
			}
		case msg := <-c.m.Data:
			if msg.Error != nil {
				return msg.Error
			}
			fmt.Printf("datarecv\n%s", hex.Dump(msg.Data))

			_, err := c.m.SendControl(ack)
			if err != nil {
				return err
			}
			fmt.Printf("ctrlsent\n%s", hex.Dump(ack))
		}
	}

	return c.m.Close()
}
