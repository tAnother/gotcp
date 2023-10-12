package proto

import (
	"net/netip"

	ipv4header "github.com/brown-csci1680/iptcp-headers"
)

const (
	MTU = 1400 // maximum-transmission-unit, default 1400 bytes

	RIPProtoNum  int = 200
	TestProtoNum int = 0
)

func ValidateCheckSum() {

	// Invert the checksum value.  Why is this necessary?
	// This function returns the inverse of the checksum
	// on an initial computation.  While this may seem weird,
	// it makes it easier to use this same function
	// to validate the checksum on the receiving side.
	// See ValidateChecksum in the receiver file for details.

	// checksumInv := checksum ^ 0xffff

	// return checksumInv
}

func ValidateChecksum(b []byte, fromHeader uint16) uint16 {
	// checksum := ipv4header.Checksum(b, fromHeader)

	// return checksum
}

func ValidateTTL() {

}

func NewHeader(srcIP netip.Addr, destIP netip.Addr, msg []byte, protoNum int) *ipv4header.IPv4Header {
	return &ipv4header.IPv4Header{
		Version:  4,
		Len:      20, // Header length is always 20 when no IP options
		TOS:      0,
		TotalLen: ipv4header.HeaderLen + len(msg),
		ID:       0,
		Flags:    0,
		FragOff:  0,
		TTL:      32,
		Protocol: protoNum,
		Checksum: 0, // Should be 0 until checksum is computed
		Src:      srcIP,
		Dst:      destIP,
		Options:  []byte{},
	}
}
