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
)

type Node struct {
	Interfaces     map[string]*Interface          // interface name -> interface instance
	IFNeighbors    map[string][]*Neighbor         // interface name -> a list of neighbors on that interface
	RoutingTable   map[netip.Prefix]*RoutingEntry // aka forwarding table
	RoutingTableMu *sync.RWMutex

	RecvHandlers   map[int]RecvHandlerFunc
	RecvHandlersMu *sync.RWMutex
	IFNeighborsMu  *sync.RWMutex
	InterfacesMu   *sync.RWMutex
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

type RoutingEntry struct {
	RouteType    string
	NextHop      netip.Addr // nil if local
	LocalNextHop string     // interface name. "" if not local
	Cost         uint16
}

// ------------ IP API ------------
//
// init a node instance & register handlers
func Init(config lnxconfig.IPConfig) (*Node, error) {
	return nil, nil
}

// listen on designated interface, receive & unmarshal packets, then forward them to specific handlers
func (n *Node) ListenOn(udpPort uint16) {

}

type RecvHandlerFunc func() // params TBD

func (n *Node) RegisterRecvHandler(protoNum uint16, callbackFunc RecvHandlerFunc) {

}

// send:
// 1. Marshal data
// 2. Recursively find next hop in the routing table
// 3. Look up for the udp port in neighbor list
// 4. Write to the dest UDP port
func (n *Node) Send(destIP netip.Addr, msg string, protoNum uint16) error {
	var nextHop *RoutingEntry
	var srcIP netip.Addr
	var remoteAddr *net.UDPAddr
	var srcUDPConn *net.UDPConn
	var err error

	switch uint16(proto.TestProtoNum) {
	case 0:
		nextHop = n.findNextHop(destIP)
		if nextHop.LocalNextHop == "" {
			return fmt.Errorf("error finding local next hop for the test packet")
		}
		// find src interface
		srcIF := n.findSrcIP(nextHop.LocalNextHop)
		srcIP = srcIF.GetAssignedIP()
		srcUDPConn = srcIF.GetUDPConn()
		// find the nbhr
		nbhr := n.findNeighbor(nextHop.LocalNextHop, destIP)
		if nbhr == nil {
			return fmt.Errorf("dest ip does not exist in neighbors")
		}
		// resolve nbhrs udp addr
		remoteAddr, err = util.RemotePort(nbhr.GetUDPAddr())
		if err != nil {
			return fmt.Errorf("error resolving the dest udp port")
		}

	case 200:
		nextHop = nil //TODO for RIP
	default:
		nextHop = nil
	}

	// Start filling in the header

	hdr := proto.IPv4Header{
		Version:  4,
		Len:      20, // Header length is always 20 when no IP options
		TOS:      0,
		TotalLen: proto.HeaderLen + len(msg),
		ID:       0,
		Flags:    0,
		FragOff:  0,
		TTL:      32,
		Protocol: 0,
		Checksum: 0, // Should be 0 until checksum is computed
		Src:      srcIP,
		Dst:      destIP,
		Options:  []byte{},
	}

	// Assemble the header into a byte array
	headerBytes, err := hdr.Marshal()
	if err != nil {
		return fmt.Errorf("error marshalling header:  %s", err)
	}

	// Cast back to an int, which is what the Header structure expects
	hdr.Checksum = int(proto.ComputeChecksum(headerBytes))

	headerBytes, err = hdr.Marshal()
	if err != nil {
		return fmt.Errorf("error marshalling header: %s", err)
	}
	bytesToSend := make([]byte, 0, len(headerBytes)+len(msg))
	bytesToSend = append(bytesToSend, headerBytes...)
	bytesToSend = append(bytesToSend, []byte(msg)...)

	// Send the message to the "link-layer" addr:port on UDP
	bytesWritten, err := srcUDPConn.WriteToUDP(bytesToSend, remoteAddr)
	if err != nil {
		log.Panicln("Error writing to socket: ", err)
	}
	fmt.Printf("Sent %d bytes\n", bytesWritten)
	return nil
}

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

	// ------- Test Packet
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
		if nbhr.GetVIPString() == destIP.String() {
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

// turn up/turn down an interface:
// 1. Update interface info
// (2. For routers: Notify rip neighbors)
func (n *Node) SetInterfaceIsDown(ifname string, down bool) error {
	n.InterfacesMu.RLock()
	defer n.InterfacesMu.RUnlock()

	iface, ok := n.Interfaces[ifname]
	if !ok {
		return fmt.Errorf("[SetInterfaceIsDown] interface %s does not exist\n", ifname)
	}
	iface.SetInterfaceIsDown(down)
	return nil
}

func updateRoutingtable() { // params TBD

}

// ------- Neighbor
func (n *Neighbor) GetVIPString() string {
	return n.VIP.String()
}

func (n *Neighbor) GetUDPString() string {
	return n.UDPAddr.Addr().String()
}

func (n *Neighbor) GetUDPPort() uint16 {
	return n.UDPAddr.Port()
}

func (n *Neighbor) GetUDPAddr() netip.AddrPort {
	return n.UDPAddr
}

// ------- Interface
func (i *Interface) GetIsDownString() string {
	if i.IsDown {
		return "down"
	}
	return "up"
}

func (i *Interface) GetPrefixString() string {
	return i.AssignedPrefix.String()
}

func (i *Interface) SetInterfaceIsDown(isDown bool) {
	i.IsDown = isDown
}

func (i *Interface) GetAssignedIP() netip.Addr {
	return i.AssignedIP
}

func (i *Interface) GetUDPConn() *net.UDPConn {
	return i.conn
}

// ------- RoutingEntry
func (rt *RoutingEntry) GetRouteTypeString() string {
	return rt.RouteType
}

func (rt *RoutingEntry) GetNextHopString() string {
	if rt.LocalNextHop != "" {
		return fmt.Sprintf("LOCAL:%s", rt.NextHop.String())
	}
	return rt.NextHop.String()
}

func (rt *RoutingEntry) GetCostString() string {
	return fmt.Sprint(rt.Cost)
}

// ------- Node Print Helpers
// Returns a string list of interface, vip of neighbor, udp of neighbor
func (n *Node) GetNeighborsString() [][]string {
	n.IFNeighborsMu.RLock()
	defer n.IFNeighborsMu.RUnlock()

	rowSize := 0
	for _, val := range n.IFNeighbors {
		rowSize += len(val)
	}

	res := make([][]string, rowSize)
	rowIndex := 0
	for key, val := range n.IFNeighbors {
		nbhrSize := len(val)
		for i := 0; i < nbhrSize; i++ {
			row := []string{key, val[i].GetVIPString(), val[i].GetUDPString()}
			res[rowIndex] = row
			rowIndex += 1
		}
	}

	return res
}

// Returns a string list of interface
func (n *Node) GetInterfacesString() [][]string {
	n.InterfacesMu.RLock()
	defer n.InterfacesMu.RUnlock()
	size := len(n.Interfaces)
	res := make([][]string, size)
	index := 0
	for key, val := range n.Interfaces {
		res[index] = []string{key, val.GetPrefixString(), val.GetIsDownString()}
		index += 1
	}
	return res
}

func (n *Node) GetRoutingTableString() [][]string {
	n.RoutingTableMu.RLock()
	defer n.RoutingTableMu.RUnlock()
	size := len(n.RoutingTable)
	res := make([][]string, size)
	index := 0
	for prefix, rt := range n.RoutingTable {
		res[index] = []string{rt.GetRouteTypeString(), prefix.String(), rt.GetNextHopString(), rt.GetCostString()}
		index += 1
	}
	return res
}
