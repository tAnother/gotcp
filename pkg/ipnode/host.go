package ipnode

import (
	"fmt"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/proto"
)

func NewHost(config *lnxconfig.IPConfig) (*Node, error) {
	host, err := newNode(config)
	if err != nil {
		return nil, err
	}
	host.RegisterRecvHandler(proto.TestProtoNum, testRecvHandler)
	return host, nil
}

func testRecvHandler(packet *proto.Packet, node *Node) {
	packet.Header.TTL--
	if packet.Header.TTL == 0 {
		return
	}
	fmt.Printf("Received test packet: Src: %s, Dst: %s, TTL: %d, Data: %s\n",
		packet.Header.Src, packet.Header.Dst, packet.Header.TTL, packet.Payload)
}
