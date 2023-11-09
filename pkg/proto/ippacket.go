package proto

import (
	"fmt"
	"net/netip"

	ipv4header "github.com/brown-csci1680/iptcp-headers"
	"github.com/google/netstack/tcpip/header"
)

const (
	MTU                = 1400 // maximum-transmission-unit, default 1400 bytes
	DefaultIpHeaderLen = ipv4header.HeaderLen

	ProtoNumRIP  uint8 = 200
	ProtoNumTest uint8 = 0
	ProtoNumTCP  uint8 = uint8(header.TCPProtocolNumber)
)

// var logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

type IPPacket struct {
	Header  *ipv4header.IPv4Header
	Payload []byte
}

// Create a new packet. msg will be truncated if packet length exceeds MTU
func NewPacket(srcIP netip.Addr, destIP netip.Addr, msg []byte, protoNum uint8) *IPPacket {
	// logger.Printf("Creating a new packet with srcIP: %v, destIP: %v, protoNum: %v, length of msg: %v", srcIP, destIP, protoNum, len(msg))
	hdr := newHeader(srcIP, destIP, msg, protoNum)
	return &IPPacket{
		Header:  hdr,
		Payload: msg[:hdr.TotalLen-hdr.Len],
	}
}

func (p *IPPacket) Marshal() ([]byte, error) {
	headerBytes, err := p.Header.Marshal()
	if err != nil {
		return nil, fmt.Errorf("error marshalling header: %s", err)
	}
	bytesToSend := make([]byte, 0, len(headerBytes)+len(p.Payload))
	bytesToSend = append(bytesToSend, headerBytes...)
	bytesToSend = append(bytesToSend, p.Payload...)
	return bytesToSend, nil
}

func (p *IPPacket) Unmarshal(data []byte) error {
	hdr, err := ipv4header.ParseHeader(data)
	if err != nil {
		return fmt.Errorf("error parsing header: %v", err)
	}
	p.Header = hdr
	p.Payload = data[hdr.Len:]
	return nil
}

func ComputeChecksum(b []byte) uint16 {
	checksum := header.Checksum(b, 0)

	// Invert the checksum value.  Why is this necessary?
	// This function returns the inverse of the checksum
	// on an initial computation.  While this may seem weird,
	// it makes it easier to use this same function
	// to validate the checksum on the receiving side.
	// See ValidateChecksum in the receiver file for details.
	checksumInv := checksum ^ 0xffff

	return checksumInv
}

func ValidateIPChecksum(b []byte, fromHeader uint16) uint16 {
	// Here, we provide both the byte array for the header AND
	// the initial checksum value that was stored in the header
	//
	// "Why don't we need to set the checksum value to 0 first?"
	//
	// Normally, the checksum is computed with the checksum field
	// of the header set to 0.  This library creatively avoids
	// this step by instead subtracting the initial value from
	// the computed checksum.
	// If you use a different language or checksum function, you may
	// need to handle this differently.
	checksum := header.Checksum(b, fromHeader)

	return checksum
}

func newHeader(srcIP netip.Addr, destIP netip.Addr, msg []byte, protoNum uint8) *ipv4header.IPv4Header {
	return &ipv4header.IPv4Header{
		Version:  4,
		Len:      DefaultIpHeaderLen, // Header length is always 20 when no IP option is provided
		TOS:      0,
		TotalLen: min(DefaultIpHeaderLen+len(msg), MTU),
		ID:       0,
		Flags:    0,
		FragOff:  0,
		TTL:      32,
		Protocol: int(protoNum),
		Checksum: 0, // Should be 0 until checksum is computed
		Src:      srcIP,
		Dst:      destIP,
		Options:  []byte{},
	}
}
