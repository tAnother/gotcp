package ipnode

import (
	"fmt"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/proto"
	"iptcp-nora-yu/pkg/util"
	"log"
	"net"
	"net/netip"
	"sync"

	ipv4header "github.com/brown-csci1680/iptcp-headers"
)

type Node struct {
	Interfaces     map[string]*Interface          // interface name -> interface instance
	IFNeighbors    map[string][]*Neighbor         // interface name -> a list of neighbors on that interface  /// considering making this private
	RoutingTable   map[netip.Prefix]*RoutingEntry // aka forwarding table
	InterfacesMu   *sync.RWMutex
	IFNeighborsMu  *sync.RWMutex
	RoutingTableMu *sync.RWMutex

	RecvHandlers map[uint8]RecvHandlerFunc
}

type Neighbor struct {
	VIP     netip.Addr // virtual ip addr
	UDPAddr netip.AddrPort
	IFName  string
}

type Interface struct {
	Name           string
	AssignedIP     netip.Addr
	AssignedPrefix netip.Prefix
	SubnetMask     uint32
	UDPAddr        netip.AddrPort
	IsDown         bool
	conn           *net.UDPConn
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

// init a node instance & register handlers
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
			SubnetMask:     uint32(ifcfg.AssignedPrefix.Bits()),
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
type RecvHandlerFunc func(packet *proto.Packet, node *Node)

func (n *Node) RegisterRecvHandler(protoNum uint8, callbackFunc RecvHandlerFunc) {
	n.RecvHandlers[protoNum] = callbackFunc
}

// listen on designated interface, receive & unmarshal packets, then forward them to specific handlers
func (n *Node) ListenOn(i *Interface) {
	listenAddr, err := net.ResolveUDPAddr("udp4", i.UDPAddr.String())
	if err != nil {
		log.Println("Error resolving UDP address: ", err)
		return
	}
	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		log.Println("Could not bind to UDP port: ", err)
		return
	}
	i.conn = conn

	for {
		buf := make([]byte, proto.MTU)
		nbytes, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Errorreading from UDP socket: ", err)
			return
		}

		// parse packet header
		hdr, err := ipv4header.ParseHeader(buf)
		if err != nil {
			log.Println("Error parsing header: ", err)
			continue // fail to parse - simply drop the packet
		}

		// validate checksum
		checksumFromHeader := uint16(hdr.Checksum)
		if checksumFromHeader != proto.ValidateChecksum(buf[:nbytes], checksumFromHeader) {
			log.Println("Checksum mismatch detected")
			continue // bad packet - simply drop it
		}

		// forward to handler
		handler, ok := n.RecvHandlers[uint8(hdr.Protocol)]
		if ok {
			packet := &proto.Packet{Header: hdr, Payload: buf[hdr.Len:]}
			handler(packet, n)
		}
	}
}

// Send:
// 1. Marshal data
// 2. Recursively find next hop in the routing table
// 3. Look up for the udp port in neighbor list
// 4. Write to the dest UDP port
func (n *Node) Send(destIP netip.Addr, msg string, protoNum uint8) error {
	var nextHop *RoutingEntry
	var srcIP netip.Addr
	var remoteAddr *net.UDPAddr
	var srcUDPConn *net.UDPConn
	var err error

	switch protoNum {
	case proto.TestProtoNum:
		nextHop = n.findNextHop(destIP)
		if nextHop.LocalNextHop == "" {
			return fmt.Errorf("error finding local next hop for the test packet")
		}
		// find src interface
		srcIF := n.findSrcIP(nextHop.LocalNextHop)
		srcIP = srcIF.AssignedIP
		srcUDPConn = srcIF.conn // TODO: null check
		// find the nbhr
		nbhr := n.findNeighbor(nextHop.LocalNextHop, destIP)
		if nbhr == nil {
			return fmt.Errorf("dest ip does not exist in neighbors")
		}
		// resolve nbhrs udp addr
		remoteAddr, err = util.RemotePort(nbhr.UDPAddr)
		if err != nil {
			return fmt.Errorf("error resolving the dest udp port")
		}

	case proto.RIPProtoNum:
		nextHop = nil //TODO for RIP
	default:
		return fmt.Errorf("invalid protocol num %d", protoNum)
	}

	// Start filling in the header
	packet := proto.NewPacket(srcIP, destIP, []byte(msg), protoNum)

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
	bytesWritten, err := srcUDPConn.WriteToUDP(bytesToSend, remoteAddr)
	if err != nil {
		return fmt.Errorf("error writing to socket: %v", err)
	}
	log.Printf("Sent %d bytes from %v to %v \n", bytesWritten, srcIP, destIP)
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
		return fmt.Errorf("[SetInterfaceIsDown] interface %s does not exist\n", ifname)
	}
	iface.setIsDown(down)
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
		res[index] = fmt.Sprintf("%s\t%s\t%s\n", ifname, i.getPrefixString(), i.getIsDownString())
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
func (n *Node) findNextHop(destIP netip.Addr) *RoutingEntry {
	// ------- Test Packet
	matchedPrefix := n.findLongestMatchedPrefix(destIP)
	n.RoutingTableMu.RLock()
	defer n.RoutingTableMu.RUnlock()

	entry := n.RoutingTable[matchedPrefix]
	// if not local next hop, need to consult the routing table again
	if entry.LocalNextHop == "" {
		matchedPrefix = n.findLongestMatchedPrefix(entry.NextHop)
		if !matchedPrefix.IsValid() {
			return nil
		}
		entry = n.RoutingTable[matchedPrefix]
	}
	return entry
}

func (n *Node) findLongestMatchedPrefix(destIP netip.Addr) netip.Prefix {
	n.RoutingTableMu.RLock()
	defer n.RoutingTableMu.RUnlock()
	var longestPrefix netip.Prefix
	maxLength := 0
	for prefix, _ := range n.RoutingTable {
		if prefix.Contains(destIP) && prefix.Bits() > maxLength {
			longestPrefix = prefix
			maxLength = prefix.Bits()
		}
	}
	return longestPrefix
}

func (n *Node) findNeighbor(ifName string, destIP netip.Addr) *Neighbor {
	n.IFNeighborsMu.RLock()
	defer n.IFNeighborsMu.RUnlock()

	nbhrs := n.IFNeighbors[ifName]

	for _, nbhr := range nbhrs {
		if nbhr.getVIPString() == destIP.String() {
			return nbhr
		}
	}
	return nil
}

func (n *Node) findSrcIP(ifName string) *Interface {
	n.InterfacesMu.RLock()
	defer n.InterfacesMu.RUnlock()
	return n.Interfaces[ifName]
}

func updateRoutingtable() { // params TBD

}

// ------- Neighbor
func (n *Neighbor) getVIPString() string {
	return n.VIP.String()
}

func (n *Neighbor) getUDPString() string {
	return n.UDPAddr.String()
}

func (n *Neighbor) GetUDPPort() uint16 {
	return n.UDPAddr.Port()
}

// ------- Interface
func (i *Interface) getIsDownString() string {
	if i.IsDown {
		return "down"
	}
	return "up"
}

func (i *Interface) getPrefixString() string {
	return i.AssignedPrefix.String()
}

func (i *Interface) setIsDown(isDown bool) { /// might just get rid of this
	i.IsDown = isDown
}

// ------- RoutingEntry
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
