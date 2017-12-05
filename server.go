package main

import (
	"encoding/hex"
	"fmt"
	"time"
)

const protocolVersion = 1

type Server struct {
	m  *Multicast
	tb *VirtualTarballReader
}

func NewServer(m *Multicast, tb *VirtualTarballReader) *Server {
	return &Server{
		m,
		tb,
	}
}

func (s *Server) controlMessage(op byte, data []byte) []byte {
	msg := make([]byte, 0, 1+32+1+len(data))
	msg = append(msg, protocolVersion)
	msg = append(msg, s.tb.HashId()...)
	msg = append(msg, op)
	msg = append(msg, data...)
	return msg
}

func (s *Server) Run() error {
	go s.m.ControlReceiveLoop()

	// Tick to send a server announcement:
	ticker := time.Tick(1 * time.Second)

	// Create an announcement message:
	announcement := s.controlMessage(byte(Announce), nil)

	// Send/recv loop:
	for {
		select {
		case ctrl := <-s.m.Control:
			if ctrl.Error != nil {
				return ctrl.Error
			}
			s.processControl(ctrl)
		case <-ticker:
			_, err := s.m.SendControl(announcement)
			if err != nil {
				return err
			}
		}
	}

	return s.m.Close()
}

func (s *Server) processControl(ctrl UDPMessage) {
	fmt.Printf("ctrlrecv\n%s", hex.Dump(ctrl.Data))
}
