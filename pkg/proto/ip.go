package proto

import (
	"net/netip"
)

const (
	Version   = 4    // protocol version
	HeaderLen = 20   // header length without extension headers
	MTU       = 1400 // maximum-transmission-unit, default 1400 bytes

	RIPProtoNum  int = 200
	TestProtoNum int = 0
)

type HeaderFlags int

const (
	MoreFragments HeaderFlags = 1 << iota // more fragments flag
	DontFragment                          // don't fragment flag
)

type IPv4Header struct {
	Version  int         // protocol version
	Len      int         // header length
	TOS      int         // type-of-service
	TotalLen int         // packet total length
	ID       int         // identification
	Flags    HeaderFlags // flags
	FragOff  int         // fragment offset
	TTL      int         // time-to-live
	Protocol int         // next protocol
	Checksum int         // checksum
	Src      netip.Addr  // source address
	Dst      netip.Addr  // destination address
	Options  []byte      // options, extension headers
}

func (h *IPv4Header) Marshal() ([]byte, error) {
	return nil, nil
}

func (h *IPv4Header) UnMarshal(b []byte) error {
	return nil
}

func ComputeCheckSum() {

}

func ValidateCheckSum() {

}

func ValidateTTL() {

}
