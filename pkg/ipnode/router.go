package ipnode

import (
	"fmt"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/proto"
	"iptcp-nora-yu/pkg/util"
	"net/netip"
	"time"
)

func NewRouter(config *lnxconfig.IPConfig) (*Node, error) {
	router, err := newNode(config)
	if err != nil {
		return nil, err
	}

	router.RegisterRecvHandler(proto.ProtoNumRIP, ripRecvHandler)
	router.RegisterRecvHandler(proto.ProtoNumTest, routerTestRecvHandler)

	return router, nil
}

func routerTestRecvHandler(packet *proto.Packet, node *Node) {
	for _, i := range node.Interfaces {
		if packet.Header.Dst == i.AssignedIP {
			fmt.Printf("Received test packet: Src: %s, Dst: %s, TTL: %d, Data: %s\n",
				packet.Header.Src, packet.Header.Dst, packet.Header.TTL, packet.Payload)
			return
		}
	}

	// forward the packet
	packet.Header.Checksum = 0
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

func ripRecvHandler(packet *proto.Packet, node *Node) {
	fmt.Printf("Received rip packet: Src: %s, Dst: %s, TTL: %d, Data: %s\n",
		packet.Header.Src, packet.Header.Dst, packet.Header.TTL, packet.Payload)

	msg := new(proto.RipMsg)
	err := msg.Unmarshal(packet.Payload)
	if err != nil {
		logger.Printf("Error unmarshaling message: %v\n", err)
		return
	}

	switch msg.Command {
	case proto.RoutingCmdTypeRequest:
		if msg.NumEntries != 0 {
			logger.Printf("RIP request must have 0 num entries.\n")
			return
		}
		responseBytes, err := node.routingTableToRipResponse()
		if err != nil {
			logger.Println(err)
			return
		}
		node.Send(packet.Header.Src, responseBytes, proto.ProtoNumRIP)

	case proto.RoutingCmdTypeResponse:
		entries := ripEntriesToRoutingEntries(msg.Entries, packet.Header.Src)
		node.updateRoutingTable(entries)

	default:
		logger.Printf("Unknown routing command type: %v\n", msg.Command)
	}
}

// Send updated routing entries to all RIP neighbors
func (n *Node) SendRipUpdate() {
	responseBytes, err := n.routingTableToRipResponse()
	if err != nil {
		logger.Println(err)
		return
	}
	for _, neighbor := range n.ripNeighbors {
		n.Send(neighbor, responseBytes, proto.ProtoNumRIP)
	}
}

// Send rip request to all neighbors. Called at router start up
func (n *Node) SendRipRequest() {
	request := proto.RipMsg{
		Command:    proto.RoutingCmdTypeRequest,
		NumEntries: 0,
	}
	requestBytes, err := request.Marshal()
	if err != nil {
		logger.Printf("Error marshaling rip msg: %v\n", err)
		return
	}
	for _, neighbor := range n.ripNeighbors {
		n.Send(neighbor, requestBytes, proto.ProtoNumRIP)
	}
}

/**************************** helper funcs ****************************/

func (n *Node) updateRoutingTable(entries []*RoutingEntry) {
	now := time.Now()
	n.RoutingTableMu.Lock()
	defer n.RoutingTableMu.Unlock()

	for _, entry := range entries {
		oldEntry, ok := n.RoutingTable[entry.Prefix]
		if ok {
			oldEntry.updatedAt = now
			oldEntry.expiryT.Reset(12 * time.Second)
			if oldEntry.NextHop == entry.NextHop { // cost update for the same route
				oldEntry.Cost = entry.Cost
			} else if entry.Cost < oldEntry.Cost { // better route found
				oldEntry.NextHop = entry.NextHop
				oldEntry.Cost = entry.Cost
			}
		} else if entry.Cost < proto.INFINITY { // new prefix
			n.RoutingTable[entry.Prefix] = entry
			entry.updatedAt = now
			entry.expiryT = time.NewTimer(12 * time.Second)

			// spawn a thread to handle expiring for this new entry
			go func(node *Node, entry *RoutingEntry) {
				<-entry.expiryT.C
				node.RoutingTableMu.Lock()
				delete(node.RoutingTable, entry.Prefix)
				node.RoutingTableMu.Unlock()
			}(n, entry)
		}
	}
}

// Create and marshal rip response
func (n *Node) routingTableToRipResponse() ([]byte, error) {
	updatedEntries := n.getUpdatedEntries()
	response := &proto.RipMsg{
		Command:    proto.RoutingCmdTypeResponse,
		NumEntries: uint16(len(updatedEntries)),
		Entries:    routingEntriesToRipEntries(updatedEntries),
	}
	responseBytes, err := response.Marshal()
	if err != nil {
		return nil, fmt.Errorf("error marshaling rip msg: %v", err)
	}
	return responseBytes, nil
}

// Get all routing entries updated after the last sent heartbeat
func (n *Node) getUpdatedEntries() []*RoutingEntry {
	var entries []*RoutingEntry
	n.RoutingTableMu.RLock()
	defer n.RoutingTableMu.RUnlock()

	lastHeartbeat := time.Now().Add(-5 * time.Second)
	for _, r := range n.RoutingTable {
		if r.updatedAt.After(lastHeartbeat) {
			entries = append(entries, r)
		}
	}
	return entries
}

func routingEntriesToRipEntries(entries []*RoutingEntry) []*proto.RipEntry {
	ripEntries := make([]*proto.RipEntry, len(entries))
	for i, r := range entries {
		ripEntries[i] = &proto.RipEntry{
			Cost:    uint32(r.Cost),
			Address: util.IpToUint32(r.Prefix.Addr()),
			Mask:    uint32(r.Prefix.Bits()),
		}
	}
	return ripEntries
}

func ripEntriesToRoutingEntries(entries []*proto.RipEntry, proposer netip.Addr) []*RoutingEntry {
	routingEntries := make([]*RoutingEntry, len(entries))
	for i, r := range entries {
		routingEntries[i] = &RoutingEntry{
			RouteType: RIP,
			Prefix:    netip.PrefixFrom(util.Uint32ToIp(r.Address), int(r.Mask)),
			NextHop:   proposer,
			Cost:      max(r.Cost+1, proto.INFINITY),
		}
	}
	return routingEntries
}
