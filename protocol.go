// protocol.go
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

const protocolVersion = 1
const protocolPrefixSize = 1 + 32 + 1

var (
	ErrMessageTooShort      = errors.New("message too short")
	ErrWrongProtocolVersion = errors.New("wrong protocol version")
	ErrAckOutOfRange        = errors.New("ack out of range")
)

var byteOrder = binary.LittleEndian

type ControlToClientOp byte
type ControlToServerOp byte

const (
	// To-Client control messages:
	AnnounceTarball = ControlToClientOp(iota)
	RespondMetadataHeader
	RespondMetadataSection
	DeliverDataSection

	// To-Server control messages:
	RequestMetadataHeader = ControlToServerOp(iota)
	RequestMetadataSection
	NakDataSection
)

type TarballFileMetadata struct {
	Path string
	Size int64
	Mode uint32
	Hash [sha256.Size]byte
}

type TarballMetadata struct {
	Files []TarballFileMetadata
	Size  int64
}

type NakRegion struct {
	start int64
	endEx int64
}

type NakRegions struct {
	naks []NakRegion
	size int64
}

func NewNakRegions(size int64) *NakRegions {
	return &NakRegions{naks: []NakRegion{{start: 0, endEx: size}}, size: size}
}

func (r *NakRegions) Naks() []NakRegion {
	return r.naks
}

func (r *NakRegions) Len() int {
	return len(r.naks)
}

func (r *NakRegions) Clear() {
	r.naks = []NakRegion{{start: 0, endEx: r.size}}
}

// [].ack(?, ?) => []
// [(0, 10)].ack(0, 10) => []
// [(0, 10)].ack(0,  5) => [(5, 10)]
// [(0, 10)].ack(5, 10) => [(0,  5)]
// [(0, 10)].ack(2,  5) => [(0,  2), (5, 10)]
func (r *NakRegions) Ack(start int64, endEx int64) error {
	if start < 0 {
		return ErrAckOutOfRange
	}
	if endEx > r.size {
		return ErrAckOutOfRange
	}

	// ACK has no effect on a fully-acked region:
	a := r.naks
	if len(a) == 0 {
		return nil
	}

	// ACK a range by creating a modified NAK ranges:
	o := make([]NakRegion, 0, len(a))
	for _, k := range a {
		if start == k.start && endEx == k.endEx {
			// remove this range from output; i.e. dont add it.
		} else if start > k.start && endEx < k.endEx {
			// [(0, 10)].ack(2,  5) => [(0,  2), (5, 10)]
			o = append(o, NakRegion{k.start, start})
			o = append(o, NakRegion{endEx, k.endEx})
		} else if start > k.start && endEx == k.endEx {
			// [(0, 10)].ack(5, 10) => [(0,  5)]
			o = append(o, NakRegion{k.start, start})
		} else if start == k.start && endEx < k.endEx {
			// [(0, 10)].ack(0,  5) => [(5, 10)]
			o = append(o, NakRegion{endEx, k.endEx})
		} else {
			o = append(o, k)
		}
	}

	r.naks = o
	return nil
}

func controlToClientMessage(hashId []byte, op ControlToClientOp, data []byte) []byte {
	msg := make([]byte, 0, 1+32+1+len(data))
	msg = append(msg, protocolVersion)
	msg = append(msg, hashId...)
	msg = append(msg, byte(op))
	msg = append(msg, data...)
	return msg
}

func controlToServerMessage(hashId []byte, op ControlToServerOp, data []byte) []byte {
	msg := make([]byte, 0, 1+32+1+len(data))
	msg = append(msg, protocolVersion)
	msg = append(msg, hashId...)
	msg = append(msg, byte(op))
	msg = append(msg, data...)
	return msg
}

func extractClientMessage(ctrl UDPMessage) (hashId []byte, op ControlToClientOp, data []byte, err error) {
	if len(ctrl.Data) < 34 {
		err = ErrMessageTooShort
		return
	}

	if ctrl.Data[0] != protocolVersion {
		err = ErrWrongProtocolVersion
		return
	}

	hashId = ctrl.Data[1:33]
	op = ControlToClientOp(ctrl.Data[33])
	data = ctrl.Data[34:]

	return
}

func extractServerMessage(ctrl UDPMessage) (hashId []byte, op ControlToServerOp, data []byte, err error) {
	if len(ctrl.Data) < 34 {
		err = ErrMessageTooShort
		return
	}

	if ctrl.Data[0] != protocolVersion {
		err = ErrWrongProtocolVersion
		return
	}

	hashId = ctrl.Data[1:33]
	op = ControlToServerOp(ctrl.Data[33])
	data = ctrl.Data[34:]

	return
}
