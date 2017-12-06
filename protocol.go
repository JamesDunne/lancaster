// protocol.go
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const protocolVersion = 1
const hashSize = 8
const protocolControlPrefixSize = 1 + hashSize + 1
const protocolDataMsgPrefixSize = 1 + hashSize + 8

const metadataSectionMsgSize = 2
const metadataHeaderMsgSize = 2

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
	AckDataSection
)

func compareHashes(a []byte, b []byte) int {
	return bytes.Compare(a[:hashSize], b[:hashSize])
}

type Region struct {
	start int64
	endEx int64
}

type NakRegions struct {
	naks []Region
	size int64
}

func NewNakRegions(size int64) *NakRegions {
	return &NakRegions{naks: []Region{{start: 0, endEx: size}}, size: size}
}

func (r *NakRegions) Naks() []Region {
	return r.naks
}

func (r *NakRegions) Len() int {
	return len(r.naks)
}

func (r *NakRegions) Clear() {
	r.naks = []Region{{start: 0, endEx: r.size}}
}

func (r *NakRegions) IsAllAcked() bool {
	return len(r.naks) == 0
}

func (r *NakRegions) NextNakRegion(start int64) int64 {
	if r.IsAllAcked() {
		return -1
	}

	for _, k := range r.naks {
		if start >= k.start && start < k.endEx {
			return start
		}
	}

	// Try the first nak region if nothing available after `start`:
	if len(r.naks) > 0 {
		return r.naks[0].start
	}

	return -1
}

func (r *NakRegions) IsAcked(start int64, endEx int64) bool {
	for _, k := range r.naks {
		if start >= k.start && endEx <= k.endEx {
			return false
		}
	}

	return true
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
	o := make([]Region, 0, len(a))
	for _, k := range a {
		if start == k.start && endEx == k.endEx {
			// remove this range from output; i.e. dont add it.
		} else if start > k.start && endEx < k.endEx {
			// [(0, 10)].ack(2,  5) => [(0,  2), (5, 10)]
			o = append(o, Region{k.start, start})
			o = append(o, Region{endEx, k.endEx})
		} else if start > k.start && endEx == k.endEx {
			// [(0, 10)].ack(5, 10) => [(0,  5)]
			o = append(o, Region{k.start, start})
		} else if start == k.start && endEx < k.endEx {
			// [(0, 10)].ack(0,  5) => [(5, 10)]
			o = append(o, Region{endEx, k.endEx})
		} else {
			o = append(o, k)
		}
	}

	r.naks = o
	return nil
}

func controlToClientMessage(hashId []byte, op ControlToClientOp, data []byte) []byte {
	msg := make([]byte, 0, protocolControlPrefixSize+len(data))
	msg = append(msg, protocolVersion)
	msg = append(msg, hashId[:hashSize]...)
	msg = append(msg, byte(op))
	msg = append(msg, data...)
	return msg
}

func controlToServerMessage(hashId []byte, op ControlToServerOp, data []byte) []byte {
	msg := make([]byte, 0, protocolControlPrefixSize+len(data))
	msg = append(msg, protocolVersion)
	msg = append(msg, hashId[:hashSize]...)
	msg = append(msg, byte(op))
	msg = append(msg, data...)
	return msg
}

func dataMessage(hashId []byte, region int64, data []byte) []byte {
	msg := make([]byte, 0, protocolDataMsgPrefixSize+len(data))
	buf := bytes.NewBuffer(msg)
	buf.WriteByte(protocolVersion)
	buf.Write(hashId[:hashSize])
	binary.Write(buf, byteOrder, region)
	buf.Write(data)
	return buf.Bytes()
}

func extractControlMessage(ctrl UDPMessage) (hashId []byte, op byte, data []byte, err error) {
	if len(ctrl.Data) < protocolControlPrefixSize {
		err = ErrMessageTooShort
		return
	}

	if ctrl.Data[0] != protocolVersion {
		err = ErrWrongProtocolVersion
		return
	}

	hashId = ctrl.Data[1 : 1+hashSize]
	op = ctrl.Data[1+hashSize]
	data = ctrl.Data[protocolControlPrefixSize:]

	return
}

func extractClientMessage(ctrl UDPMessage) (hashId []byte, op ControlToClientOp, data []byte, err error) {
	var opByte byte
	hashId, opByte, data, err = extractControlMessage(ctrl)
	op = ControlToClientOp(opByte)
	return
}

func extractServerMessage(ctrl UDPMessage) (hashId []byte, op ControlToServerOp, data []byte, err error) {
	var opByte byte
	hashId, opByte, data, err = extractControlMessage(ctrl)
	op = ControlToServerOp(opByte)
	return
}

func extractDataMessage(ctrl UDPMessage) (hashId []byte, region int64, data []byte, err error) {
	if len(ctrl.Data) < protocolDataMsgPrefixSize {
		err = ErrMessageTooShort
		return
	}

	if ctrl.Data[0] != protocolVersion {
		err = ErrWrongProtocolVersion
		return
	}

	hashId = ctrl.Data[1 : 1+hashSize]
	region = int64(byteOrder.Uint64(ctrl.Data[1+hashSize : protocolDataMsgPrefixSize]))
	data = ctrl.Data[protocolDataMsgPrefixSize:]

	return
}
