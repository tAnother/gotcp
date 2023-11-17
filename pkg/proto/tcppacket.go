package proto

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"strings"

	"github.com/google/netstack/tcpip/header"
)

const (
	DefaultTcpHeaderLen = header.TCPMinimumSize
	TcpPseudoHeaderLen  = 12
	MSS                 = MTU - DefaultTcpHeaderLen - DefaultIpHeaderLen
)

type TCPPacket struct {
	TcpHeader *header.TCPFields
	Payload   []byte
}

func NewTCPacket(localPort uint16, destPort uint16, seqNum uint32, ackNum uint32, flags uint8, payload []byte, windowSize uint16) *TCPPacket {
	tcpHdr := &header.TCPFields{
		SrcPort:       localPort,
		DstPort:       destPort, // Header length is always 20 when no IP option is provided
		SeqNum:        seqNum,
		AckNum:        ackNum,
		DataOffset:    DefaultTcpHeaderLen,
		Flags:         flags,
		Checksum:      0,
		UrgentPointer: 0,
		WindowSize:    windowSize,
	}
	return &TCPPacket{TcpHeader: tcpHdr, Payload: payload}
}

func (p *TCPPacket) Unmarshal(data []byte) {
	hdr := ParseTCPHeader(data)
	p.TcpHeader = &hdr
	p.Payload = data[hdr.DataOffset:]
}

// The TCP checksum is computed based on a "pesudo-header" that
// combines the (virtual) IP source and destination address, protocol value,
// as well as the TCP header and payload
//
// This is one example one is way to combine all of this information
// and compute the checksum leveraging the netstack package.
//
// For more details, see the "Checksum" component of RFC9293 Section 3.1,
// https://www.rfc-editor.org/rfc/rfc9293.txt
func ComputeTCPChecksum(tcpHdr *header.TCPFields,
	sourceIP netip.Addr, destIP netip.Addr, payload []byte) uint16 {

	// Fill in the pseudo header
	pseudoHeaderBytes := make([]byte, TcpPseudoHeaderLen)

	// First are the source and dest IPs.  This function only supports
	// IPv4, so make sure the IPs are IPv4 addresses
	copy(pseudoHeaderBytes[0:4], sourceIP.AsSlice())
	copy(pseudoHeaderBytes[4:8], destIP.AsSlice())

	// Next, add the protocol number and header length
	pseudoHeaderBytes[8] = uint8(0)
	pseudoHeaderBytes[9] = uint8(ProtoNumTCP)

	totalLength := DefaultTcpHeaderLen + len(payload)
	binary.BigEndian.PutUint16(pseudoHeaderBytes[10:12], uint16(totalLength))

	// Turn the TcpFields struct into a byte array
	headerBytes := header.TCP(make([]byte, DefaultTcpHeaderLen))
	headerBytes.Encode(tcpHdr)

	// Compute the checksum for each individual part and combine To combine the
	// checksums, we leverage the "initial value" argument of the netstack's
	// checksum package to carry over the value from the previous part
	pseudoHeaderChecksum := header.Checksum(pseudoHeaderBytes, 0)
	headerChecksum := header.Checksum(headerBytes, pseudoHeaderChecksum)
	fullChecksum := header.Checksum(payload, headerChecksum)

	// Return the inverse of the computed value,
	// which seems to be the convention of the checksum algorithm
	// in the netstack package's implementation
	return fullChecksum ^ 0xffff
}

// Build a TCPFields struct from the TCP byte array
//
// NOTE: the netstack package might have other options for parsing the header
// that you may like better--this example is most similar to our other class
// examples.  Your mileage may vary!
func ParseTCPHeader(b []byte) header.TCPFields {
	td := header.TCP(b)
	return header.TCPFields{
		SrcPort:    td.SourcePort(),
		DstPort:    td.DestinationPort(),
		SeqNum:     td.SequenceNumber(),
		AckNum:     td.AckNumber(),
		DataOffset: td.DataOffset(),
		Flags:      td.Flags(),
		WindowSize: td.WindowSize(),
		Checksum:   td.Checksum(),
	}
}

// Pretty-print TCP flags value as a string
func TCPFlagsAsString(flags uint8) string {
	strMap := map[uint8]string{
		header.TCPFlagAck: "ACK",
		header.TCPFlagFin: "FIN",
		header.TCPFlagPsh: "PSH",
		header.TCPFlagRst: "RST",
		header.TCPFlagSyn: "SYN",
		header.TCPFlagUrg: "URG",
	}

	matches := make([]string, 0)

	for b, str := range strMap {
		if (b & flags) == b {
			matches = append(matches, str)
		}
	}

	ret := strings.Join(matches, "+")

	return ret
}

// Pretty-print a TCP header (with pretty-printed flags)
// Otherwise, using %+v in format strings is a good enough view in most cases
func TCPFieldsToString(hdr *header.TCPFields) string {
	return fmt.Sprintf("{SrcPort:%d DstPort:%d, SeqNum:%d AckNum:%d DataOffset:%d Flags:%s WindowSize:%d Checksum:%x UrgentPointer:%d}",
		hdr.SrcPort, hdr.DstPort, hdr.SeqNum, hdr.AckNum, hdr.DataOffset, TCPFlagsAsString(hdr.Flags), hdr.WindowSize, hdr.Checksum, hdr.UrgentPointer)
}

func ValidTCPChecksum(p *TCPPacket, srcIp netip.Addr, dstIp netip.Addr) bool {
	tcpChecksumFromHeader := p.TcpHeader.Checksum
	p.TcpHeader.Checksum = 0
	tcpComputedChecksum := ComputeTCPChecksum(p.TcpHeader, srcIp, dstIp, p.Payload)
	return tcpComputedChecksum == tcpChecksumFromHeader
}

/************************************ TCP Packet Helpers ***********************************/

func (p *TCPPacket) IsAck() bool {
	return p.TcpHeader.Flags&header.TCPFlagAck != 0
}

func (p *TCPPacket) IsSyn() bool {
	return p.TcpHeader.Flags&header.TCPFlagSyn != 0
}

func (p *TCPPacket) IsFin() bool {
	return p.TcpHeader.Flags&header.TCPFlagFin != 0
}

func (p *TCPPacket) IsRst() bool {
	return p.TcpHeader.Flags&header.TCPFlagRst != 0
}
