package main

import (
	"encoding/hex"
	"fmt"
	"time"
)

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

func (s *Server) Run() error {
	err := (error)(nil)
	defer func() {
		err = s.m.Close()
	}()

	s.m.SendsControlToClient()
	s.m.SendsData()
	s.m.ListensControlToServer()

	// Tick to send a server announcement:
	ticker := time.Tick(1 * time.Second)

	// Create an announcement message:
	announceMsg := controlToClientMessage(s.tb.HashId(), AnnounceTarball, nil)

	// Send/recv loop:
	for {
		select {
		case ctrl := <-s.m.ControlToServer:
			if ctrl.Error != nil {
				return ctrl.Error
			}
			s.processControl(ctrl)
		case <-ticker:
			_, err := s.m.SendControlToClient(announceMsg)
			if err != nil {
				return err
			}
		}
	}

	return err
}

func (s *Server) processControl(ctrl UDPMessage) {
	fmt.Printf("ctrlrecv\n%s", hex.Dump(ctrl.Data))
}
