package ipnode

import "iptcp-nora-yu/pkg/lnxconfig"

type Router struct {
	Node        Node
	RoutingType lnxconfig.RoutingMode
	MsgChan     chan string
}

// send routing info to RIP neighbors
func SendUpdate()

// and some function to convert proto.RIPMsg into RoutingEntry's:
