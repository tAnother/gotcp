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

	ipv4header "github.com/brown-csci1680/iptcp-headers"
)

var logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

type RecvHandlerFunc func(packet *proto.Packet, node *Node)

type Node struct {
	Interfaces     map[string]*Interface          // interface name -> interface instance
	IFNeighbors    map[string][]*Neighbor         // interface name -> a list of neighbors on that interface  /// considering making this private
	RoutingTable   map[netip.Prefix]*RoutingEntry // aka forwarding table
	InterfacesMu   sync.RWMutex
	IFNeighborsMu  sync.RWMutex
	RoutingTableMu sync.RWMutex

	RecvHandlers map[uint8]RecvHandlerFunc
}

type Interface struct {
	Name           string
	AssignedIP     netip.Addr
	AssignedPrefix netip.Prefix
	SubnetMask     int
	UDPAddr        netip.AddrPort
	IsDown         bool
	conn           *net.UDPConn
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
	NextHop      netip.Addr // nil if local
	LocalNextHop string     // interface name. "" if not local
	Cost         int16
}

// Init a node instance & register handlers
func newNode(config *lnxconfig.IPConfig) (*Node, error) {
	node := &Node{
		Interfaces:   make(map[string]*Interface),
		IFNeighbors:  make(map[string][]*Neighbor),
		RoutingTable: make(map[netip.Prefix]*RoutingEntry),
		RecvHandlers: make(map[uint8]RecvHandlerFunc),
	}

	for _, ifcfg := range config.Interfaces {
		node.Interfaces[ifcfg.Name] = &Interface{
			Name:           ifcfg.Name,
			AssignedIP:     ifcfg.AssignedIP,
			AssignedPrefix: ifcfg.AssignedPrefix,
			SubnetMask:     ifcfg.AssignedPrefix.Bits(),
			UDPAddr:        ifcfg.UDPAddr,
		}

		node.IFNeighbors[ifcfg.Name] = make([]*Neighbor, 0)

		node.RoutingTable[ifcfg.AssignedPrefix] = &RoutingEntry{
			RouteType:    Local,
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
		node.IFNeighbors[ncfg.InterfaceName] = append(node.IFNeighbors[ncfg.InterfaceName], neighbor)
	}

	for prefix, addr := range config.StaticRoutes {
		node.RoutingTable[prefix] = &RoutingEntry{
			RouteType: Static,
			NextHop:   addr,
			Cost:      -1, /// there might be better way to represent cost "-". trying to think of some way to associate infinity/maxcost(16) with certain representation...
		}
	}

	return node, nil
}

/****************** IP API ******************/

func (n *Node) RegisterRecvHandler(protoNum uint8, callbackFunc RecvHandlerFunc) {
	n.RecvHandlers[protoNum] = callbackFunc
}

// Listen on designated interface, receive & unmarshal packets, then dispatch them to specific handlers
func (n *Node) ListenOn(i *Interface) {
	listenAddr := net.UDPAddrFromAddrPort(i.UDPAddr)
	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		logger.Println("Could not bind to UDP port: ", err)
		return
	}
	i.conn = conn

	for {
		buf := make([]byte, proto.MTU)
		_, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			logger.Println("Errorreading from UDP socket: ", err)
			return
		}

		// parse packet header
		hdr, err := ipv4header.ParseHeader(buf)
		if err != nil {
			logger.Println("Error parsing header: ", err)
			continue // fail to parse - simply drop the packet
		}

		// validate checksum
		checksumFromHeader := uint16(hdr.Checksum)
		if checksumFromHeader != proto.ValidateChecksum(buf[:hdr.Len], checksumFromHeader) {
			logger.Println("Checksum mismatch detected")
			continue // bad packet - simply drop it
		}

		// forward to handler
		handler, ok := n.RecvHandlers[uint8(hdr.Protocol)]
		if ok {
			packet := &proto.Packet{Header: hdr, Payload: buf[hdr.Len:]}
			handler(packet, n)
		} else {
			logger.Println("Cannot handle packet with protocol num: ", hdr.Protocol)
		}
	}
}

// Send msg to destIP
func (n *Node) Send(destIP netip.Addr, msg string, protoNum uint8) error {
	var srcIF *Interface
	var srcIP netip.Addr
	var remoteAddr netip.AddrPort

	switch protoNum {
	case proto.TestProtoNum:
		nextHop, altDestIP := n.findNextHop(destIP)
		if nextHop == nil || nextHop.LocalNextHop == "" {
			return fmt.Errorf("error finding local next hop for the test packet")
		}
		if !altDestIP.IsValid() {
			altDestIP = destIP
		}
		// find src interface
		srcIF := n.findSrcIF(nextHop.LocalNextHop)
		srcIP = srcIF.AssignedIP
		// find the nbhr
		nbhr := n.findNextNeighbor(nextHop.LocalNextHop, altDestIP)
		if nbhr == nil {
			return fmt.Errorf("dest ip does not exist in neighbors")
		}
		remoteAddr = nbhr.UDPAddr

	case proto.RIPProtoNum:
		// nextHop := nil //TODO for RIP
	default:
		return fmt.Errorf("invalid protocol num %d", protoNum)
	}

	// create a new packet for msg
	packet := proto.NewPacket(srcIP, destIP, []byte(msg), protoNum)

	return n.forwardPacket(srcIF, remoteAddr, packet)
}

// Send packet to neighbor on interface srcIF
func (n *Node) forwardPacket(srcIF *Interface, remoteAddr netip.AddrPort, packet *proto.Packet) error {
	if srcIF.conn == nil {
		listenAddr := net.UDPAddrFromAddrPort(srcIF.UDPAddr)
		conn, err := net.ListenUDP("udp4", listenAddr)
		if err != nil {
			return err
		}
		srcIF.conn = conn
	}
	udpAddr := net.UDPAddrFromAddrPort(remoteAddr)

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
	bytesWritten, err := srcIF.conn.WriteToUDP(bytesToSend, udpAddr)
	if err != nil {
		return fmt.Errorf("error writing to socket: %v", err)
	}
	logger.Printf("Sent %d bytes to %v \n", bytesWritten, udpAddr)
	return nil
}

// Turn up/turn down an interface:
// 1. Update interface info
// TODO: For routers: Notify rip neighbors (maybe we need 2 versions)
func (n *Node) SetInterfaceIsDown(ifname string, down bool) error {
	n.InterfacesMu.RLock()
	defer n.InterfacesMu.RUnlock()

	iface, ok := n.Interfaces[ifname]
	if !ok {
		return fmt.Errorf("[SetInterfaceIsDown] interface %s does not exist", ifname)
	}
	iface.IsDown = down
	return nil
}

/************ Node Print Helpers (return interface/neighbor/routing info in strings) ************/

// Returns a string list of interface, vip of neighbor, udp of neighbor
func (n *Node) GetNeighborsString() []string {
	n.IFNeighborsMu.RLock()
	defer n.IFNeighborsMu.RUnlock()

	size := 0
	for _, neighbors := range n.IFNeighbors {
		size += len(neighbors)
	}

	res := make([]string, size)
	index := 0
	for ifname, neighbors := range n.IFNeighbors {
		for _, neighbor := range neighbors {
			res[index] = fmt.Sprintf("%s\t%s\t%s\n", ifname, neighbor.getVIPString(), neighbor.getUDPString())
			index += 1
		}
	}
	return res
}

// Returns a string list of interface
func (n *Node) GetInterfacesString() []string {
	n.InterfacesMu.RLock()
	defer n.InterfacesMu.RUnlock()
	size := len(n.Interfaces)
	res := make([]string, size)
	index := 0
	for ifname, i := range n.Interfaces {
		res[index] = fmt.Sprintf("%s\t%s\t%s\n", ifname, i.getAddrPrefixString(), i.getIsDownString())
		index += 1
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

/************ helper funcs ************/

// Return the next hop & the virtual IP one step before the next hop (as the alternative link layer dest)
func (n *Node) findNextHop(destIP netip.Addr) (entry *RoutingEntry, altAddr netip.Addr) {
	// ------- Test Packet
	altAddr = netip.Addr{}
	n.RoutingTableMu.RLock()
	defer n.RoutingTableMu.RUnlock()

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
		if prefix.Contains(destIP) && prefix.Bits() > maxLength {
			longestPrefix = prefix
			maxLength = prefix.Bits()
		}
	}
	return longestPrefix
}

func (n *Node) findNextNeighbor(ifName string, nexthopIP netip.Addr) *Neighbor {
	n.IFNeighborsMu.RLock()
	defer n.IFNeighborsMu.RUnlock()

	nbhrs := n.IFNeighbors[ifName]

	for _, nbhr := range nbhrs {
		if nbhr.VIP == nexthopIP {
			return nbhr
		}
	}
	return nil
}

func (n *Node) findSrcIF(ifName string) *Interface {
	n.InterfacesMu.RLock()
	defer n.InterfacesMu.RUnlock()
	return n.Interfaces[ifName]
}

func updateRoutingtable() { // params TBD

}

func (i *Interface) getIsDownString() string {
	if i.IsDown {
		return "down"
	}
	return "up"
}

func (i *Interface) getAddrPrefixString() string {
	return fmt.Sprintf("%v/%v", i.AssignedIP, i.SubnetMask)
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
	if rt.Cost == -1 {
		return "-"
	}
	return fmt.Sprint(rt.Cost)
}
