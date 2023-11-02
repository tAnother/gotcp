package vhost

import (
	"fmt"
	"iptcp-nora-yu/pkg/ipnode"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/proto"
	"net/netip"

	"github.com/google/netstack/tcpip/header"
)

type VHost struct {
	Node *ipnode.Node
}

func New(config *lnxconfig.IPConfig) (*VHost, error) {
	node, err := ipnode.New(config)
	if err != nil {
		return nil, err
	}
	node.RegisterRecvHandler(proto.ProtoNumTest, testRecvHandler)
	return &VHost{node}, nil
}

func testRecvHandler(packet *proto.Packet, node *ipnode.Node) {
	fmt.Printf("Received test packet: Src: %s, Dst: %s, TTL: %d, Data: %s\n",
		packet.Header.Src, packet.Header.Dst, packet.Header.TTL, packet.Payload)
}

func (v *VHost) GetLocalAddr() netip.Addr {
	for _, i := range v.Node.Interfaces {
		return i.AssignedIP
	}
	return netip.Addr{}
}

func (h *VHost) SendTcpPacket(p *proto.TCPPacket, srcIP netip.Addr, destIP netip.Addr) error {
	p.TcpHeader.Checksum = proto.ComputeTCPChecksum(p.TcpHeader, srcIP, destIP, p.Payload)
	tcpHeaderBytes := make(header.TCP, proto.TcpHeaderLen)
	tcpHeaderBytes.Encode(p.TcpHeader)
	return h.Node.Send(destIP, tcpHeaderBytes, uint8(proto.ProtoNumTCP))
}
