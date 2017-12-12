// protocol.go
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
)

const protocolVersion = 1
const hashSize = 8
const protocolControlPrefixSize = 1 + hashSize + 1
const protocolDataMsgPrefixSize = 1 + hashSize + 8

const metadataSectionMsgSize = 2
const metadataHeaderMsgSize = 2

const bufferFullTimeoutMilli = 50

var resendTimeout = 500 * time.Millisecond

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

func (r *NakRegions) Acks() []Region {
	a := r.naks[:]
	o := make([]Region, 0, len(a))
	// [(0 2) (4 5) (10 19)] -> [(2, 4), (5, 10), (19, 20)]
	// [(0 20)] -> []
	m := int64(0)
	for _, k := range a {
		if k.start > m {
			o = append(o, Region{m, k.start})
		}
		m = k.endEx
	}
	if m < r.size {
		o = append(o, Region{m, r.size})
	}
	return o
}

func (r *NakRegions) Len() int {
	return len(r.naks)
}

func (r *NakRegions) NakAll() {
	r.naks = []Region{{start: 0, endEx: r.size}}
}

func (r *NakRegions) IsAllAcked() bool {
	return len(r.naks) == 0
}

func (r *NakRegions) NextNakRegion(p int64) int64 {
	if r.IsAllAcked() {
		return -1
	}

	a := r.naks[:]
	for i := 0; i < len(a); i++ {
		if a[i].start <= p && p < a[i].endEx {
			return p
		}
		if p < a[i].start {
			return a[i].start
		}
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

func (r *NakRegions) Ack(start, endEx int64) error {
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

	o := make([]Region, 0, len(a))
	kWithStart := 0
	kWithEnd := len(a) - 1
	for i := len(a) - 1; i >= 0; i-- {
		k := &a[i]
		if start >= k.start {
			kWithStart = i
			break
		}
	}
	for i := 0; i < len(a); i++ {
		k := &a[i]
		if endEx <= k.endEx {
			kWithEnd = i
			break
		}
	}

	// Condense range:
	if start < a[kWithStart].start {
		start = a[kWithStart].start
	}
	if endEx > a[kWithEnd].endEx {
		endEx = a[kWithEnd].endEx
	}

	// Emit unmodified NAK ranges before the requested ACK range:
	for i := 0; i < kWithStart; i++ {
		o = append(o, a[i])
	}

	//fmt.Printf("(%v %v) vs. (%v %v)\n", a[kWithStart].start, a[kWithEnd].endEx, start, endEx)
	if start == a[kWithStart].start && endEx == a[kWithEnd].endEx {
	} else if start == a[kWithStart].start && endEx < a[kWithEnd].endEx {
		if endEx > a[kWithEnd].start {
			o = append(o, Region{endEx, a[kWithEnd].endEx})
		} else {
			o = append(o, a[kWithEnd])
		}
	} else if start > a[kWithStart].start && endEx < a[kWithEnd].endEx {
		// [(0 1) (2 5) (6 20)].ack(3, 4) -> [(0 1) (2 3) (4 5) (6 20)]
		if start < a[kWithStart].endEx {
			o = append(o, Region{a[kWithStart].start, start})
		} else {
			o = append(o, a[kWithStart])
		}
		if endEx > a[kWithEnd].start {
			o = append(o, Region{endEx, a[kWithEnd].endEx})
		} else {
			o = append(o, a[kWithEnd])
		}
	} else if start > a[kWithStart].start && endEx == a[kWithEnd].endEx {
		if start < a[kWithEnd].endEx {
			o = append(o, Region{a[kWithStart].start, start})
		} else {
			o = append(o, a[kWithStart])
		}
	} else {
		fmt.Printf("\bWARNING! %v v %v\n", Region{start: start, endEx: endEx}, Region{a[kWithStart].start, a[kWithEnd].endEx})
	}
	// Emit unmodified NAK ranges above requested NAK range:
	for i := kWithEnd + 1; i < len(a); i++ {
		o = append(o, a[i])
	}

	r.naks = o
	return nil
}

func (r *NakRegions) Nak(start, endEx int64) error {
	if start < 0 {
		return ErrAckOutOfRange
	}
	if endEx > r.size {
		return ErrAckOutOfRange
	}

	a := r.naks
	// [].nak(0, 10) => [(0, 10)]
	if len(a) == 0 {
		r.naks = make([]Region, 1)
		r.naks[0].start = start
		r.naks[0].endEx = endEx
		return nil
	}

	// [(5, 10)].nak(0,  10) => [(0, 10)]
	// [(5, 10)].nak(0,  15) => [(0, 15)]
	// [(5, 10), (15, 20)].nak(2,  12) => [(2, 12), (15, 20)]
	// [(5, 10), (15, 20)].nak(0,  15) => [(0, 20)]

	o := make([]Region, 0, len(a))
	kWithStart := 0
	kWithEnd := 0
	for i := len(a) - 1; i >= 0; i-- {
		k := &a[i]
		if endEx >= k.start {
			kWithEnd = i
			break
		}
	}
	for i := 0; i < len(a); i++ {
		k := &a[i]
		if start <= k.endEx {
			kWithStart = i
			break
		}
	}
	// Emit unmodified NAK ranges before the requested NAK range:
	for i := 0; i < kWithStart; i++ {
		o = append(o, a[i])
	}
	// Emit requested NAK range (extended to fit with existing NAKs):
	if a[kWithStart].start < start {
		start = a[kWithStart].start
	}
	if a[kWithEnd].endEx > endEx {
		endEx = a[kWithEnd].endEx
	}
	o = append(o, Region{start, endEx})
	// Emit unmodified NAK ranges above requested NAK range:
	for i := kWithEnd + 1; i < len(a); i++ {
		o = append(o, a[i])
	}

	r.naks = o
	return nil
}

func (r *NakRegions) asciiMeter(charSize float64, nakMeter []byte) {
	for i := 0; i < len(nakMeter); i++ {
		nakMeter[i] = '#'
	}
	for _, k := range r.naks {
		i := int(math.Floor(float64(k.start) / charSize))
		ir := int(math.Ceil(float64(k.start) / charSize))
		j := int(math.Floor(float64(k.endEx) / charSize))
		jr := int(math.Ceil(float64(k.endEx) / charSize))

		for n := i; n < j && n < len(nakMeter); n++ {
			nakMeter[n] = '.'
		}

		if charSize > 1.0 {
			if i != ir {
				nakMeter[i] = ':'
			}
			if j != jr {
				nakMeter[j] = ':'
			}
		}
	}
}

func (r *NakRegions) ASCIIMeter(nakMeterLen int) string {
	charSize := float64(r.size) / float64(nakMeterLen)
	nakMeter := make([]byte, nakMeterLen)
	r.asciiMeter(charSize, nakMeter)
	return string(nakMeter)
}

func (r *NakRegions) ASCIIMeterPosition(nakMeterLen int, pos int64) string {
	charSize := float64(r.size) / float64(nakMeterLen)
	nakMeter := make([]byte, nakMeterLen)
	r.asciiMeter(charSize, nakMeter)

	i := int(math.Floor(float64(pos) / charSize))
	j := int(math.Floor(float64(pos+1) / charSize))

	for ; i <= j && i < nakMeterLen; i++ {
		nakMeter[i] = '|'
	}

	return string(nakMeter)

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
