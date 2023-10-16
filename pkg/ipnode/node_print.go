package ipnode

import "fmt"

// List (interface, assigned prefix, up/down state) as strings
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

// List (interface, vip of neighbor, udp of neighbor) as strings
func (n *Node) GetNeighborsString() []string {
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

// List (route type, prefix, nexthop) as strings
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
