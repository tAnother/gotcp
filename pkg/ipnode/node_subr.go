package ipnode

import (
	"iptcp-nora-yu/pkg/proto"
	"net"
	"net/netip"
)

// Set up UDP socket for all interfaces
func (n *Node) bindUDP() {
	for _, i := range n.Interfaces {
		listenAddr := net.UDPAddrFromAddrPort(i.UDPAddr)
		conn, err := net.ListenUDP("udp4", listenAddr)
		if err != nil {
			logger.Printf("Could not bind to UDP port: %v\n", err)
			return
		}
		i.conn = conn
		logger.Printf("Listening on interface %v with udp %v ...\n", i.Name, i.conn.LocalAddr().String())
	}
}

// Listen on designated interface, receive & unmarshal packets, then dispatch them to specific handlers
func (n *Node) listenOn(i *Interface) {
	for {
		buf := make([]byte, proto.MTU)
		_, _, err := i.conn.ReadFromUDP(buf)
		if err != nil {
			logger.Printf("Error reading from UDP socket: %v\n", err)
			return
		}
		if i.isDown { // spin wait. there should be better way to do this
			continue
		}

		p := new(proto.Packet)
		err = p.Unmarshal(buf)
		if err != nil {
			logger.Printf("Error unmarshaling packet: %v\n", err)
			continue
		}

		p.Header.TTL--

		// if packet is not for this interface and TTL reaches 0, drop the packet
		if p.Header.Dst != i.AssignedIP && p.Header.TTL <= 0 {
			logger.Printf("Packet is not for interface with IP %v and TTL reaches 0. Dropping the packet...\n", i.AssignedIP)
			continue
		}

		// validate checksum
		checksumFromHeader := uint16(p.Header.Checksum)
		if checksumFromHeader != proto.ValidateChecksum(buf[:p.Header.Len], checksumFromHeader) {
			logger.Printf("Checksum mismatch detected. Dropping the packet...\n")
			continue
		}

		// forward to handler
		handler, ok := n.recvHandlers[uint8(p.Header.Protocol)]
		if ok {
			handler(p, n)
		} else {
			logger.Printf("Handler for protocol num: %d not found\n", p.Header.Protocol)
		}
	}
}

// Return the next hop, and the virtual IP one step before the next hop (as the alternative link layer dest)
func (n *Node) findNextHopEntry(destIP netip.Addr) (entry *RoutingEntry, altAddr netip.Addr) {
	n.RoutingTableMu.RLock()
	defer n.RoutingTableMu.RUnlock()

	altAddr = netip.Addr{}
	matchedPrefix := n.findLongestMatchedPrefix(destIP)
	if !matchedPrefix.IsValid() {
		return nil, altAddr
	}
	entry = n.RoutingTable[matchedPrefix]

	// search recursively until hitting a local interface
	for entry.LocalNextHop == "" {
		altAddr = entry.NextHop
		matchedPrefix = n.findLongestMatchedPrefix(entry.NextHop)
		if !matchedPrefix.IsValid() {
			return nil, altAddr
		}
		entry = n.RoutingTable[matchedPrefix]
	}
	return entry, altAddr
}

func (n *Node) findLongestMatchedPrefix(destIP netip.Addr) netip.Prefix {
	var longestPrefix netip.Prefix
	maxLength := 0
	for prefix := range n.RoutingTable {
		if prefix.Contains(destIP) && prefix.Bits() >= maxLength {
			longestPrefix = prefix
			maxLength = prefix.Bits()
		}
	}
	return longestPrefix
}

// Return the neighbor corresponding to the given next hop IP
func (n *Node) findNextHopNeighbor(ifName string, nexthopIP netip.Addr) *Neighbor {
	nbhrs := n.IFNeighbors[ifName]

	for _, nbhr := range nbhrs {
		if nbhr.VIP == nexthopIP {
			return nbhr
		}
	}
	return nil
}
