package proto

import (
	"fmt"
	"net/netip"

)

type Packet struct {
	Header  *IPv4Header
	Payload []byte
}

func (p *Packet) Marshal() ([]byte, error) {
	headerBytes, err := p.Header.Marshal()
	if err != nil {
		return nil, fmt.Errorf("error marshalling header: %s", err)
	}
	bytesToSend := make([]byte, 0, len(headerBytes)+len(p.Payload))
	bytesToSend = append(bytesToSend, headerBytes...)
	bytesToSend = append(bytesToSend, p.Payload...)
	return bytesToSend, nil
}

func NewPacket(srcIP netip.Addr, destIP netip.Addr, msg []byte, protoNum int) *Packet {
	return &Packet{
		Header:  NewHeader(srcIP, destIP, msg, protoNum),
		Payload: msg,
	}
}
