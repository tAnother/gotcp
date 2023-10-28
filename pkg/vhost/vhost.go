package vhost

import (
	"fmt"
	"iptcp-nora-yu/pkg/ipnode"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/proto"
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
