package vrouter

import (
	"fmt"
	"iptcp-nora-yu/pkg/ipnode"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/proto"
	"log"
	"os"
	"time"
)

var logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

type VRouter struct {
	Node *ipnode.Node
}

func New(config *lnxconfig.IPConfig) (*VRouter, error) {
	node, err := ipnode.New(config)
	if err != nil {
		return nil, err
	}
	router := &VRouter{node}
	node.RegisterRecvHandler(proto.ProtoNumRIP, ripRecvHandler(router))
	node.RegisterRecvHandler(proto.ProtoNumTest, routerTestRecvHandler)
	return router, nil
}

func routerTestRecvHandler(packet *proto.Packet, node *ipnode.Node) {
	for _, i := range node.Interfaces {
		if packet.Header.Dst == i.AssignedIP {
			fmt.Printf("Received test packet: Src: %s, Dst: %s, TTL: %d, Data: %s\n",
				packet.Header.Src, packet.Header.Dst, packet.Header.TTL, packet.Payload)
			return
		}
	}

	// forward the packet
	packet.Header.Checksum = 0
	// logger.Println("Forwarding packet...")

	srcIF, remoteAddr, err := node.FindLinkLayerSrcDst(packet.Header.Dst)
	if err != nil {
		logger.Println(err)
		return
	}
	err = node.ForwardPacket(srcIF, remoteAddr, packet)
	if err != nil {
		logger.Println(err)
	}
}

func ripRecvHandler(router *VRouter) func(*proto.Packet, *ipnode.Node) {
	return func(packet *proto.Packet, node *ipnode.Node) {
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
			responseBytes, err := createRipResponse(router.getAllEntries(), packet.Header.Src)
			if err != nil {
				logger.Println(err)
				return
			}
			node.Send(packet.Header.Src, responseBytes, proto.ProtoNumRIP)

		case proto.RoutingCmdTypeResponse:
			entries := ripEntriesToRoutingEntries(msg.Entries, packet.Header.Src)
			router.updateRoutingTable(entries)

		default:
			logger.Printf("Unknown routing command type: %v\n", msg.Command)
		}
	}
}

// Send rip request to all neighbors. Called at router start up
func (r *VRouter) SendRipRequest() {
	request := proto.RipMsg{
		Command:    proto.RoutingCmdTypeRequest,
		NumEntries: 0,
	}
	requestBytes, err := request.Marshal()
	if err != nil {
		logger.Printf("Error marshaling rip msg: %v\n", err)
		return
	}
	for _, neighbor := range r.Node.RipNeighbors {
		r.Node.Send(neighbor, requestBytes, proto.ProtoNumRIP)
	}
}

// Send periodic updates to all RIP neighbors
func (r *VRouter) SendPeriodicRipUpdate() {
	r.sendRipUpdate(r.getAllEntries())
}

// Remove expried entries from the routing table and send update to RIP neighbors
func (r *VRouter) RemoveExpiredEntries() {
	var updated []*ipnode.RoutingEntry
	r.Node.RoutingTableMu.Lock()

	expiry := time.Now().Add(-12 * time.Second)
	for prefix, entry := range r.Node.RoutingTable {
		if entry.RouteType == ipnode.RouteTypeRIP && entry.UpdatedAt.Before(expiry) {
			// logger.Printf("removing %v from table...\n", prefix)
			entry.Cost = proto.INFINITY
			updated = append(updated, entry)
			delete(r.Node.RoutingTable, prefix)
		}
	}

	r.Node.RoutingTableMu.Unlock()
	r.sendRipUpdate(updated)
}

func (r *VRouter) updateRoutingTable(entries []*ipnode.RoutingEntry) {
	now := time.Now()
	var updated []*ipnode.RoutingEntry
	r.Node.RoutingTableMu.Lock()

	for _, entry := range entries {
		oldEntry, ok := r.Node.RoutingTable[entry.Prefix]
		if ok && oldEntry.RouteType != ipnode.RouteTypeRIP { // we do not care about local or static routes
			continue
		}
		if ok {
			if oldEntry.NextHop == entry.NextHop { // same route heart beat
				oldEntry.UpdatedAt = now
				if oldEntry.Cost != entry.Cost { // cost update for the same route
					oldEntry.Cost = entry.Cost
					updated = append(updated, oldEntry)
				}
			} else if entry.Cost < oldEntry.Cost { // better route found
				oldEntry.UpdatedAt = now
				oldEntry.NextHop = entry.NextHop
				oldEntry.Cost = entry.Cost
				updated = append(updated, oldEntry)
			}
		} else if entry.Cost < proto.INFINITY { // new prefix
			r.Node.RoutingTable[entry.Prefix] = entry
			entry.UpdatedAt = now
			updated = append(updated, entry)
		}
	}

	r.Node.RoutingTableMu.Unlock()
	r.sendRipUpdate(updated)
}

// Send RIP update containing entries to all RIP neighbors
func (r *VRouter) sendRipUpdate(entries []*ipnode.RoutingEntry) {
	if len(entries) == 0 {
		return
	}
	for _, neighbor := range r.Node.RipNeighbors {
		responseBytes, err := createRipResponse(entries, neighbor)
		if err != nil {
			logger.Println(err)
		}
		r.Node.Send(neighbor, responseBytes, proto.ProtoNumRIP)
	}
}

// All local & remote entries in the routing table
func (r *VRouter) getAllEntries() []*ipnode.RoutingEntry {
	var entries []*ipnode.RoutingEntry
	r.Node.RoutingTableMu.RLock()
	defer r.Node.RoutingTableMu.RUnlock()
	for _, entry := range r.Node.RoutingTable {
		if entry.RouteType == ipnode.RouteTypeRIP || entry.RouteType == ipnode.RouteTypeLocal {
			entries = append(entries, entry)
		}
	}
	return entries
}
