package ipnode

import (
	"fmt"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/proto"
	"log"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	ipv4header "github.com/brown-csci1680/iptcp-headers"
)

var logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

type RecvHandlerFunc func(packet *proto.Packet, node *Node)

type Node struct {
	Interfaces     map[string]*Interface          // interface name -> interface instance
	ifNeighbors    map[string][]*Neighbor         // interface name -> a list of neighbors on that interface
	RoutingTable   map[netip.Prefix]*RoutingEntry // aka forwarding table
	RoutingTableMu sync.RWMutex

	routingMode  lnxconfig.RoutingMode
	recvHandlers map[uint8]RecvHandlerFunc

	// router specific
	ripNeighbors []netip.Addr
}

type Interface struct {
	Name           string
	AssignedIP     netip.Addr
	AssignedPrefix netip.Prefix
	UDPAddr        netip.AddrPort
	isDown         bool

	conn *net.UDPConn // the udp socket conn used by this interface
}

type Neighbor struct {
	VIP     netip.Addr // virtual ip addr
	UDPAddr netip.AddrPort
	IFName  string
}

type RouteType string

const (
	Local  RouteType = "L"
	Static RouteType = "S"
	RIP    RouteType = "R"
)

type RoutingEntry struct {
	RouteType    RouteType
	Prefix       netip.Prefix
	NextHop      netip.Addr // 0 if local
	LocalNextHop string     // interface name. "" if not local
	Cost         uint32

	UpdatedAt time.Time
}

// Init a node instance & register handlers
func newNode(config *lnxconfig.IPConfig) (*Node, error) {
	node := &Node{
		Interfaces:   make(map[string]*Interface),
		ifNeighbors:  make(map[string][]*Neighbor),
		RoutingTable: make(map[netip.Prefix]*RoutingEntry),
		recvHandlers: make(map[uint8]RecvHandlerFunc),
		routingMode:  config.RoutingMode,
		ripNeighbors: make([]netip.Addr, len(config.RipNeighbors)),
	}
	copy(node.ripNeighbors, config.RipNeighbors)
	if node.routingMode == lnxconfig.RoutingTypeNone {
		node.routingMode = lnxconfig.RoutingTypeStatic
	}

	for _, ifcfg := range config.Interfaces {
		node.Interfaces[ifcfg.Name] = &Interface{
			Name:           ifcfg.Name,
			AssignedIP:     ifcfg.AssignedIP,
			AssignedPrefix: ifcfg.AssignedPrefix,
			UDPAddr:        ifcfg.UDPAddr,
		}

		node.ifNeighbors[ifcfg.Name] = make([]*Neighbor, 0)

		node.RoutingTable[ifcfg.AssignedPrefix] = &RoutingEntry{
			RouteType:    Local,
			Prefix:       ifcfg.AssignedPrefix,
			LocalNextHop: ifcfg.Name,
			Cost:         0,
		}
	}

	for _, ncfg := range config.Neighbors {
		neighbor := &Neighbor{
			VIP:     ncfg.DestAddr,
			UDPAddr: ncfg.UDPAddr,
			IFName:  ncfg.InterfaceName,
		}
		node.ifNeighbors[ncfg.InterfaceName] = append(node.ifNeighbors[ncfg.InterfaceName], neighbor)
	}

	for prefix, addr := range config.StaticRoutes {
		node.RoutingTable[prefix] = &RoutingEntry{
			RouteType: Static,
			Prefix:    prefix,
			NextHop:   addr,
			Cost:      0,
		}
	}

	return node, nil
}

/************************************ IP API ***********************************/

func (n *Node) RegisterRecvHandler(protoNum uint8, callbackFunc RecvHandlerFunc) {
	n.recvHandlers[protoNum] = callbackFunc
}

func (n *Node) BindUDP() {
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
func (n *Node) ListenOn(i *Interface) {
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

		// parse packet header
		hdr, err := ipv4header.ParseHeader(buf)
		if err != nil {
			logger.Printf("Error parsing header: %v. Dropping the packet...\n", err)
			continue
		}

		hdr.TTL--
		// logger.Printf("Received packet: Src: %s, Dst: %s, TTL: %d, ProtoNum: %v\n", hdr.Src, hdr.Dst, hdr.TTL, hdr.Protocol)

		// if packet is not for this interface and TTL reaches 0, drop the packet
		if !isForInterface(i, hdr) && hdr.TTL <= 0 {
			logger.Printf("Packet is not for interface with IP %v and TTL reaches 0. Dropping the packet...\n", i.AssignedIP)
			continue
		}

		// validate checksum
		checksumFromHeader := uint16(hdr.Checksum)
		if checksumFromHeader != proto.ValidateChecksum(buf[:hdr.Len], checksumFromHeader) {
			logger.Printf("Checksum mismatch detected. Dropping the packet...\n")
			continue
		}

		// forward to handler
		handler, ok := n.recvHandlers[uint8(hdr.Protocol)]
		if ok {
			packet := &proto.Packet{Header: hdr, Payload: buf[hdr.Len:]}
			handler(packet, n)
		} else {
			logger.Printf("Handler for protocol num: %d not found\n", hdr.Protocol)
		}
	}
}

// Send msg to destIP
func (n *Node) Send(destIP netip.Addr, msg []byte, protoNum uint8) error {
	if protoNum != proto.ProtoNumRIP && protoNum != proto.ProtoNumTest {
		return fmt.Errorf("invalid protocol num %d", protoNum)
	}

	srcIF, remoteAddr, err := n.findLinkLayerSrcDst(destIP)
	if err != nil {
		return err
	}
	packet := proto.NewPacket(srcIF.AssignedIP, destIP, msg, protoNum)
	err = n.forwardPacket(srcIF, remoteAddr, packet)
	return err
}

// Turn up/turn down an interface:
func (n *Node) SetInterfaceIsDown(ifname string, down bool) error {
	iface, ok := n.Interfaces[ifname]
	if !ok {
		return fmt.Errorf("[SetInterfaceIsDown] interface %s does not exist", ifname)
	}
	iface.isDown = down
	return nil
}

/**************** Node Print Helpers (return interface/neighbor/routing info in strings) ****************/

// Returns a string list of interface
func (n *Node) GetInterfacesString() []string {
	size := len(n.Interfaces)
	res := make([]string, size)
	index := 0
	for ifname, i := range n.Interfaces {
		res[index] = fmt.Sprintf("%s\t%s\t%s\n", ifname, i.getAddrPrefixString(), i.getIsDownString())
		index += 1
	}
	return res
}

// Returns a string list of interface, vip of neighbor, udp of neighbor
func (n *Node) GetNeighborsString() []string {
	size := 0
	for _, neighbors := range n.ifNeighbors {
		size += len(neighbors)
	}

	res := make([]string, size)
	index := 0
	for ifname, neighbors := range n.ifNeighbors {
		for _, neighbor := range neighbors {
			res[index] = fmt.Sprintf("%s\t%s\t%s\n", ifname, neighbor.getVIPString(), neighbor.getUDPString())
			index += 1
		}
	}
	return res
}

func (n *Node) GetRoutingTableString() []string {
	n.RoutingTableMu.RLock()
	defer n.RoutingTableMu.RUnlock()
	size := len(n.RoutingTable)
	res := make([]string, size)
	index := 0
	for prefix, rt := range n.RoutingTable {
		res[index] = fmt.Sprintf("%s\t%s\t%s\t%s\n", string(rt.RouteType), prefix.String(), rt.getNextHopString(), rt.getCostString())
		index += 1
	}
	return res
}

/**************************** helper funcs ****************************/

// Send packet to neighbor on interface srcIF
func (n *Node) forwardPacket(srcIF *Interface, dst netip.AddrPort, packet *proto.Packet) error {
	if srcIF.isDown {
		logger.Printf("srcIF %v is down. Dropping packet...\n", srcIF.Name)
		return nil
	}
	if srcIF.conn == nil {
		// logger.Printf("Initializing a udp connection on interface %v...\n", srcIF.Name)
		listenAddr := net.UDPAddrFromAddrPort(srcIF.UDPAddr)
		conn, err := net.ListenUDP("udp4", listenAddr)
		if err != nil {
			return err
		}
		srcIF.conn = conn
	}
	udpAddr := net.UDPAddrFromAddrPort(dst)

	// Assemble the header into a byte array
	headerBytes, err := packet.Header.Marshal()
	if err != nil {
		return fmt.Errorf("error marshalling header:  %s", err)
	}

	// Cast back to an int, which is what the Header structure expects
	packet.Header.Checksum = int(proto.ComputeChecksum(headerBytes))

	bytesToSend, err := packet.Marshal()
	if err != nil {
		return fmt.Errorf("error marshalling packet: %s", err)
	}

	// Send the message to the "link-layer" addr:port on UDP
	_, err = srcIF.conn.WriteToUDP(bytesToSend, udpAddr)
	if err != nil {
		return fmt.Errorf("error writing to socket: %v", err)
	}
	// logger.Printf("Sent %d bytes from %v(%v) to %v\n", bytesWritten, srcIF.AssignedIP, srcIF.Name, udpAddr)
	return nil
}

// Return the link layer src (interface) and dest (remote addr)
func (n *Node) findLinkLayerSrcDst(destIP netip.Addr) (*Interface, netip.AddrPort, error) {
	nextHop, altDestIP := n.findNextHopEntry(destIP)
	// logger.Printf("Next hop: %v; previous step (alternative destIP): <%v>\n", nextHop, altDestIP)
	if nextHop == nil || nextHop.LocalNextHop == "" {
		return nil, netip.AddrPort{}, fmt.Errorf("error finding local next hop for the test packet")
	}
	if !altDestIP.IsValid() {
		altDestIP = destIP
	}
	srcIF := n.Interfaces[nextHop.LocalNextHop]
	// if it's for this local interface
	if srcIF.AssignedIP == destIP {
		return srcIF, srcIF.UDPAddr, nil
	}

	// if it's for the nbhr of this local interface, find the neighbor
	nbhr := n.findNextHopNeighbor(nextHop.LocalNextHop, altDestIP)
	if nbhr == nil {
		return nil, netip.AddrPort{}, fmt.Errorf("destIP not found in the neighbors of %v", nextHop.LocalNextHop)
	}
	return srcIF, nbhr.UDPAddr, nil
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
	n.RoutingTableMu.RLock()
	defer n.RoutingTableMu.RUnlock()
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
	nbhrs := n.ifNeighbors[ifName]

	for _, nbhr := range nbhrs {
		if nbhr.VIP == nexthopIP {
			return nbhr
		}
	}
	return nil
}

func (i *Interface) getIsDownString() string {
	if i.isDown {
		return "down"
	}
	return "up"
}

func (i *Interface) getAddrPrefixString() string {
	return fmt.Sprintf("%v/%v", i.AssignedIP, i.AssignedPrefix.Bits())
}

func (n *Neighbor) getVIPString() string {
	return n.VIP.String()
}

func (n *Neighbor) getUDPString() string {
	return n.UDPAddr.String()
}

func (rt *RoutingEntry) getNextHopString() string {
	if rt.LocalNextHop != "" {
		return fmt.Sprintf("LOCAL:%s", rt.LocalNextHop)
	}
	return rt.NextHop.String()
}

func (rt *RoutingEntry) getCostString() string {
	return fmt.Sprint(rt.Cost)
}

func isForInterface(i *Interface, hdr *ipv4header.IPv4Header) bool {
	return hdr.Dst == i.AssignedIP
}
