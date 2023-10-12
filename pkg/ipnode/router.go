package ipnode

import (
	"fmt"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/proto"
	"net/netip"
)

type Router struct {
	Node         Node
	RoutingMode  lnxconfig.RoutingMode
	RipNeighbors []netip.Addr
}

func NewRouter(config *lnxconfig.IPConfig) (*Router, error) {
	node, err := newNode(config)
	if err != nil {
		return nil, err
	}

	router := &Router{
		Node:         *node,
		RoutingMode:  config.RoutingMode,
		RipNeighbors: make([]netip.Addr, len(config.RipNeighbors)),
	}
	copy(router.RipNeighbors, config.RipNeighbors)

	node.RegisterRecvHandler(proto.RIPProtoNum, ripRecvHandler)

	return router, nil
}

func ripRecvHandler(packet *proto.Packet) {
	fmt.Printf("Received rip packet: Src: %s, Dst: %s, TTL: %d, Data: %s\n",
		packet.Header.Src, packet.Header.Dst, packet.Header.TTL, packet.Payload)
}

// send routing info to RIP neighbors
func SendUpdate()

// and some function to convert proto.RIPMsg into RoutingEntry's:
