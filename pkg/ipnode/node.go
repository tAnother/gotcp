package ipnode

import (
	"fmt"
	"iptcp-nora-yu/pkg/lnxconfig"
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

func (n *Neighbor) GetVIPString() string {
	return n.VIP.String()
}

func (n *Neighbor) GetUDPString() string {
	return n.UDPAddr.Addr().String()
}

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

// Returns a string list of interface, vip of neighbor, udp of neighbor
func (n *Node) GetNeighborsString() [][]string {
	n.IFNeighborsMu.RLock()
	defer n.IFNeighborsMu.RUnlock()
	neighbors := n.IFNeighbors

	rowSize := 0
	for _, val := range neighbors {
		rowSize += len(val)
	}

	res := make([][]string, rowSize)
	rowIndex := 0
	for key, val := range neighbors {
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
	interfaces := n.Interfaces
	size := len(interfaces)
	res := make([][]string, size)
	index := 0
	for key, val := range interfaces {
		res[index] = []string{key, val.GetPrefixString(), val.GetIsDownString()}
		index += 1
	}
	return res
}

func (n *Node) GetRoutingTableString() [][]string {
	n.RoutingTableMu.RLock()
	defer n.RoutingTableMu.RUnlock()
}
