package ipnode

import (
	"iptcp-nora-yu/pkg/lnxconfig"
	"net/netip"
	"sync"
)

type Node struct {
	Interfaces     map[string]*Interface          // interface name -> interface instance
	IFNeighbors    map[string][]*Neighbor         // interface name -> a list of neighbors on that interface
	RoutingTable   map[netip.Prefix]*RoutingEntry // aka forwarding table
	RoutingTableMu *sync.Mutex

	RecvHandlers   map[int]RecvHandlerFunc
	RecvHandlersMu *sync.Mutex
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
}

type RoutingEntry struct {
	RouteType    uint16
	NextHop      netip.Addr // nil if local
	LocalNextHop string     // interface name. nil if not local
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
func (n *Node) Send(destIP netip.Addr, data []byte, protoNum uint16) {

}

func findNextHop(destIP netip.Addr) {

}

// turn up/turn down an interface:
// 1. Update interface info
// 2. Update the routing table
// (3. For routers: Notify rip neighbors)
func (n *Node) SetInterfaceIsDown(ifname string, down bool) {

}

func updateRoutingtable() { // params TBD

}
