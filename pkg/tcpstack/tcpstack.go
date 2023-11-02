package tcpstack

import (
	"fmt"
	"iptcp-nora-yu/pkg/ipnode"
	"iptcp-nora-yu/pkg/proto"
	"iptcp-nora-yu/pkg/vhost"
	"log"
	"net/netip"
	"os"
	"sync"

	"github.com/google/netstack/tcpip/header"
)

var tcb *VTCPGlobalInfo

var socketId int32 = -1

var logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

// TODO: Placeholder
const (
	BUFFER_CAPACITY = 1 << 8
)

type State string

type TCPEndpointID struct {
	localAddr  netip.Addr
	localPort  uint16
	remoteAddr netip.Addr
	remotePort uint16
}

type VTCPGlobalInfo struct {
	LocalAddr     netip.Addr
	ListenerTable map[uint16]*VTCPListener //port num: listener
	ConnTable     map[TCPEndpointID]*VTCPConn
	TableMu       sync.RWMutex
	Driver        *vhost.VHost
}

type VTCPListener struct {
	socketId      int32
	addr          netip.Addr
	port          uint16
	pendingSocket chan struct {
		*proto.TCPPacket
		netip.Addr
	} //since we need info about TCP header and the srcIP
}

type VTCPConn struct { //represents a TCP socket
	socketId   int32
	localAddr  netip.Addr
	localPort  uint16
	remoteAddr netip.Addr
	remotePort uint16

	sendBuff *proto.CircBuff
	recvBuff *proto.CircBuff

	state   State
	stateMu sync.Mutex

	srtt             *proto.SRTT
	localInitSeqNum  uint32 //should be atomic.
	remoteInitSeqNum uint32
	largestAckedNum  uint32 //should be atomic. the largest ACK we received
	expectedSeqNum   uint32 //should be atomic. the ACK num we should send to the other side

	windowSize uint32

	recvChan chan *proto.TCPPacket //see details of VRead in the handout.
}

func Init(addr netip.Addr, host *vhost.VHost) {
	tcb = &VTCPGlobalInfo{
		ListenerTable: make(map[uint16]*VTCPListener),
		ConnTable:     make(map[TCPEndpointID]*VTCPConn),
		TableMu:       sync.RWMutex{},
		LocalAddr:     addr,
		Driver:        host,
	}
	host.Node.RegisterRecvHandler(uint8(proto.ProtoNumTCP), tcpRecvHandler)
}

/************************************ TCP API ***********************************/
// VListen creates a new listening socket bound to the specified port
func VListen(port uint16) (*VTCPListener, error) {
	//1. check the listener table to see if the port is already in use
	if isPortInUse(port) {
		return nil, fmt.Errorf("port %v already in use", port)
	}
	// 2. If not, create a new listener socket
	addr := tcb.LocalAddr
	if !addr.IsValid() {
		return nil, fmt.Errorf("error finding the local addr")
	}
	l := NewListenerSocket(port, addr)
	bindListener(port, l)
	fmt.Printf("Created a listener socket with id %v\n", l.socketId)
	return l, nil
}

func VConnect(addr netip.Addr, port uint16) (*VTCPConn, error) {
	//1. create a new endpoint ID with a random port number
	endpoint := TCPEndpointID{
		localAddr:  tcb.LocalAddr,
		localPort:  generateRandomPortNum(),
		remoteAddr: addr,
		remotePort: port,
	}

	//2.check if the conn is in the table
	if _, ok := tcb.ConnTable[endpoint]; ok {
		return nil, fmt.Errorf("error connecting. The socket with local port %v, remote addr %v and port %v already exists", endpoint.localPort, addr, port)
	}

	//3. create the socket
	conn := NewSocket(SYN_SENT, endpoint, 0)
	bindSocket(endpoint, conn)

	//4. create and send syn tcp packet
	newTcpPacket := proto.NewTCPacket(endpoint.localPort, endpoint.remotePort,
		conn.localInitSeqNum, 0,
		header.TCPFlagSyn, make([]byte, 0))
	err := tcb.Driver.SendTcpPacket(newTcpPacket, endpoint.localAddr, endpoint.remoteAddr)
	if err != nil {
		//TODO: should we delete the socket from the table????
		deleteSocket(endpoint)
		return nil, fmt.Errorf("error sending SYN packet from %v to %v", netip.AddrPortFrom(endpoint.localAddr, endpoint.localPort), netip.AddrPortFrom(addr, port))
	}

	//5. state machine: syn_rent --> established
	err = handleSynSent(conn, endpoint)
	if err != nil {
		return nil, err
	}
	go conn.handleConnection()
	fmt.Printf("Created a new socket with id %v\n", conn.socketId)
	return conn, nil
}

/************************************ TCP Recv Handler ***********************************/
func tcpRecvHandler(packet *proto.Packet, node *ipnode.Node) {
	tcpPacket := new(proto.TCPPacket)
	tcpPacket.Unmarshal(packet.Payload[:packet.Header.TotalLen-packet.Header.Len])
	if valid := proto.ValidateTCPChecksum(tcpPacket, packet.Header.Src, packet.Header.Dst); !valid {
		logger.Printf("packet dropped because checksum validation failed")
		return
	}
	fmt.Printf("Received TCP packet from %s\tIP Header:  %v\tTCP header:  %+v\tFlags:  %s\tPayload (%d bytes):  %s\n",
		packet.Header.Src, packet.Header, tcpPacket.TcpHeader, proto.TCPFlagsAsString(tcpPacket.TcpHeader.Flags), len(tcpPacket.Payload), string(tcpPacket.Payload))

	if tcb.LocalAddr != packet.Header.Dst {
		logger.Printf("packet dropped because the packet with dst %v is not for %v.", packet.Header.Dst, tcb.LocalAddr)
		return
	}

	//matching process
	endpoint := TCPEndpointID{
		localAddr:  tcb.LocalAddr,
		localPort:  tcpPacket.TcpHeader.DstPort,
		remoteAddr: packet.Header.Src,
		remotePort: tcpPacket.TcpHeader.SrcPort,
	}
	forwardPacket(endpoint, tcpPacket)
}

func forwardPacket(endpoint TCPEndpointID, packet *proto.TCPPacket) {
	tcb.TableMu.RLock()
	defer tcb.TableMu.RUnlock()

	if s, ok := tcb.ConnTable[endpoint]; !ok {
		l, ok := tcb.ListenerTable[endpoint.localPort]
		if !ok {
			logger.Printf("packet dropped because there's no matching listener and normal sockets for port %v.", endpoint.localPort)
			return
		}
		l.pendingSocket <- struct {
			*proto.TCPPacket
			netip.Addr
		}{packet, endpoint.remoteAddr}
	} else {
		s.recvChan <- packet
	}
}
