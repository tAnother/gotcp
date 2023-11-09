package ipstack

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
)

var logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

type IPGlobalInfo struct {
	Interfaces     map[string]*Interface          // interface name -> interface instance
	IFNeighbors    map[string][]*Neighbor         // interface name -> a list of neighbors on that interface
	RoutingTable   map[netip.Prefix]*RoutingEntry // aka forwarding table
	RoutingTableMu sync.RWMutex

	routingMode  lnxconfig.RoutingMode
	recvHandlers map[uint8]func(*proto.IPPacket)

	// router specific
	RipNeighbors []netip.Addr
}

type Interface struct {
	Name           string
	AssignedIP     netip.Addr
	AssignedPrefix netip.Prefix
	UDPAddr        netip.AddrPort

	isDown bool
	conn   *net.UDPConn // the udp socket conn used by this interface
}

type Neighbor struct {
	VIP     netip.Addr // virtual ip addr
	UDPAddr netip.AddrPort
	IFName  string
}

type RouteType string

const (
	RouteTypeLocal  RouteType = "L"
	RouteTypeStatic RouteType = "S"
	RouteTypeRIP    RouteType = "R"
)

type RoutingEntry struct {
	RouteType    RouteType
	Prefix       netip.Prefix
	NextHop      netip.Addr // 0 if local
	LocalNextHop string     // interface name. "" if not local
	Cost         uint32

	UpdatedAt time.Time
}

// Init a node instance
func Init(config *lnxconfig.IPConfig) (*IPGlobalInfo, error) {
	s := &IPGlobalInfo{
		Interfaces:   make(map[string]*Interface),
		IFNeighbors:  make(map[string][]*Neighbor),
		RoutingTable: make(map[netip.Prefix]*RoutingEntry),
		recvHandlers: make(map[uint8]func(*proto.IPPacket)),
		routingMode:  config.RoutingMode,
		RipNeighbors: make([]netip.Addr, len(config.RipNeighbors)),
	}
	copy(s.RipNeighbors, config.RipNeighbors)
	if s.routingMode == lnxconfig.RoutingTypeNone {
		s.routingMode = lnxconfig.RoutingTypeStatic
	}

	for _, ifcfg := range config.Interfaces {
		s.Interfaces[ifcfg.Name] = &Interface{
			Name:           ifcfg.Name,
			AssignedIP:     ifcfg.AssignedIP,
			AssignedPrefix: ifcfg.AssignedPrefix,
			UDPAddr:        ifcfg.UDPAddr,
		}

		s.IFNeighbors[ifcfg.Name] = make([]*Neighbor, 0)

		s.RoutingTable[ifcfg.AssignedPrefix] = &RoutingEntry{
			RouteType:    RouteTypeLocal,
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
		s.IFNeighbors[ncfg.InterfaceName] = append(s.IFNeighbors[ncfg.InterfaceName], neighbor)
	}

	for prefix, addr := range config.StaticRoutes {
		s.RoutingTable[prefix] = &RoutingEntry{
			RouteType: RouteTypeStatic,
			Prefix:    prefix,
			NextHop:   addr,
			Cost:      0,
		}
	}

	return s, nil
}

/************************************ IP API ***********************************/

// Start the node
func (s *IPGlobalInfo) Start() {
	s.bindUDP()
	for _, i := range s.Interfaces {
		go s.listenOn(i)
	}
}

func (s *IPGlobalInfo) RegisterRecvHandler(protoNum uint8, callback func(*proto.IPPacket)) {
	s.recvHandlers[protoNum] = callback
}

// Turn up/turn down an interface
func (s *IPGlobalInfo) SetInterfaceIsDown(ifname string, down bool) error {
	iface, ok := s.Interfaces[ifname]
	if !ok {
		return fmt.Errorf("[SetInterfaceIsDown] interface %s does not exist", ifname)
	}
	iface.isDown = down
	return nil
}

// Send msg to destIP
func (s *IPGlobalInfo) Send(destIP netip.Addr, msg []byte, protoNum uint8) error {
	if protoNum != proto.ProtoNumRIP && protoNum != proto.ProtoNumTest && protoNum != proto.ProtoNumTCP {
		return fmt.Errorf("invalid protocol num %d", protoNum)
	}

	srcIF, remoteAddr, err := s.FindLinkLayerSrcDst(destIP)
	if err != nil {
		return err
	}
	packet := proto.NewPacket(srcIF.AssignedIP, destIP, msg, protoNum)
	err = ForwardPacket(srcIF, remoteAddr, packet)
	return err
}

// Send packet to neighbor on interface srcIF
func ForwardPacket(srcIF *Interface, dst netip.AddrPort, packet *proto.IPPacket) error {
	if srcIF.isDown {
		return nil
	}
	if srcIF.conn == nil {
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
	return nil
}

// Return the link layer src (interface) and dest (remote addr)
func (s *IPGlobalInfo) FindLinkLayerSrcDst(destIP netip.Addr) (*Interface, netip.AddrPort, error) {
	nextHop, altDestIP := s.findNextHopEntry(destIP)
	if nextHop == nil || nextHop.LocalNextHop == "" {
		return nil, netip.AddrPort{}, fmt.Errorf("error finding local next hop for the test packet")
	}
	if !altDestIP.IsValid() {
		altDestIP = destIP
	}
	srcIF := s.Interfaces[nextHop.LocalNextHop]
	// if it's for this local interface
	if srcIF.AssignedIP == destIP {
		return srcIF, srcIF.UDPAddr, nil
	}

	// if it's for the nbhr of this local interface, find the neighbor
	nbhr := s.findNextHopNeighbor(nextHop.LocalNextHop, altDestIP)
	if nbhr == nil {
		return nil, netip.AddrPort{}, fmt.Errorf("destIP not found in the neighbors of %v", nextHop.LocalNextHop)
	}
	return srcIF, nbhr.UDPAddr, nil
}
