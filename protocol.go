// protocol.go
package main

import (
	"errors"
)

const protocolVersion = 1

var (
	ErrMessageTooShort      = errors.New("message too short")
	ErrWrongProtocolVersion = errors.New("wrong protocol version")
)

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
