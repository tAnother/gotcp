package ipnode

import (
	"fmt"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/proto"
	"net/netip"
)

type Router struct {
	Node         *Node
	RoutingMode  lnxconfig.RoutingMode
	RipNeighbors []netip.Addr
}

func NewRouter(config *lnxconfig.IPConfig) (*Router, error) {
	node, err := newNode(config)
	if err != nil {
		return nil, err
	}

	router := &Router{
		Node:         node,
		RoutingMode:  config.RoutingMode,
		RipNeighbors: make([]netip.Addr, len(config.RipNeighbors)),
	}
	copy(router.RipNeighbors, config.RipNeighbors)

	node.RegisterRecvHandler(proto.RIPProtoNum, ripRecvHandler)
	node.RegisterRecvHandler(proto.TestProtoNum, routerTestRecvHandler)

	return router, nil
}

func ripRecvHandler(packet *proto.Packet, node *Node) {
	fmt.Printf("Received rip packet: Src: %s, Dst: %s, TTL: %d, Data: %s\n",
		packet.Header.Src, packet.Header.Dst, packet.Header.TTL, packet.Payload)
}

func routerTestRecvHandler(packet *proto.Packet, node *Node) {
	for _, i := range node.Interfaces {
		if packet.Header.Dst == i.AssignedIP {
			fmt.Printf("Received test packet: Src: %s, Dst: %s, TTL: %d, Data: %s\n",
				packet.Header.Src, packet.Header.Dst, packet.Header.TTL, packet.Payload)
			return
		}
	}

	logger.Printf("Received test packet: Src: %s, Dst: %s, TTL: %d, Data: %s\n",
		packet.Header.Src, packet.Header.Dst, packet.Header.TTL, packet.Payload)

	// try to forward the packet
	packet.Header.Checksum = 0
	packet.Header.TTL = packet.Header.TTL - 1
	if packet.Header.TTL == 0 {
		return
	}
	logger.Println("Forwarding packet...")

	srcIF, remoteAddr, err := node.findLinkLayerSrcDst(packet.Header.Dst)
	if err != nil {
		logger.Println(err)
		return
	}
	err = node.forwardPacket(srcIF, remoteAddr, packet)
	if err != nil {
		logger.Println(err)
	}
}

// send routing info to RIP neighbors
func SendUpdate() {}

// and some function to convert proto.RIPMsg into RoutingEntry's:
