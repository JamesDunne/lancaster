package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

type Server struct {
	m  *Multicast
	tb *VirtualTarballReader

	metadataHeader   []byte
	metadataSections [][]byte
}

const metadataSectionMsgPrefixSize = 36

func NewServer(m *Multicast, tb *VirtualTarballReader) *Server {
	return &Server{
		m:  m,
		tb: tb,
	}
}

func (s *Server) Run() error {
	err := (error)(nil)
	defer func() {
		err = s.m.Close()
	}()

	// Construct metadata sections:
	{
		tb := s.tb
		mdSize := (2 + 8) + (len(tb.files) * (2 + 40 + 8 + 4 + 32))
		mdBuf := bytes.NewBuffer(make([]byte, 0, mdSize))

		byteOrder := binary.LittleEndian
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
		writeBytes := func(b []byte) {
			if err == nil {
				_, err = mdBuf.Write(b)
			}
		}

		writePrimitive(tb.size)
		writePrimitive(uint32(len(tb.files)))
		for _, f := range tb.files {
			writeString(f.Path)
			writePrimitive(f.Size)
			writePrimitive(f.Mode)
			writeBytes(f.Hash)
		}
		if err != nil {
			return err
		}

		md := mdBuf.Bytes()
		fmt.Printf("md\n%s", hex.Dump(md))

		sectionSize := (s.m.datagramSize - protocolPrefixSize - metadataSectionMsgPrefixSize)
		sectionCount := len(md) / sectionSize
		if sectionCount*sectionSize < len(md) {
			sectionCount++
		}

		// Slice into sections:
		s.metadataSections = make([][]byte, 0, sectionCount)
		o := 0
		for n := 0; n < sectionCount; n++ {
			l := o + sectionSize
			if l > len(md) {
				l = len(md) - o
			}
			s.metadataSections = append(s.metadataSections, md[o:l])
			o += l
		}
		fmt.Printf("%v\n", s.metadataSections)
	}

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

func (s *Server) processControl(ctrl UDPMessage) error {
	fmt.Printf("ctrlrecv\n%s", hex.Dump(ctrl.Data))
	hashId, op, data, err := extractServerMessage(ctrl)
	if err != nil {
		return err
	}

	if bytes.Compare(hashId, s.tb.HashId()) != 0 {
		// Ignore message not for us:
		return nil
	}

	switch op {
	case RequestMetadataHeader:
		_ = data

		// Compose metadata header and send to clients:
		s.m.SendControlToClient(controlToClientMessage(hashId, RespondMetadataHeader, s.metadataHeader))
	}

	return nil
}
