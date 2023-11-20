package tcpstack

import (
	"errors"
	"fmt"
	"iptcp-nora-yu/pkg/ipstack"
	"iptcp-nora-yu/pkg/proto"
	"time"

	"log"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"

	deque "github.com/gammazero/deque"
)

var logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile) // TODO: provide config

type TCPEndpointID struct {
	LocalAddr  netip.Addr
	LocalPort  uint16
	RemoteAddr netip.Addr
	RemotePort uint16
}

type TCPGlobalInfo struct {
	IP        *ipstack.IPGlobalInfo
	localAddr netip.Addr

	listenerTable map[uint16]*VTCPListener // key: port num
	connTable     map[TCPEndpointID]*VTCPConn
	tableMu       sync.RWMutex // TODO: make it finer grained?

	socketNum int32 // a counter that keeps track of the most recent socket id

	// TODO: add extra maps for faster querying by socket id?
}

type VTCPListener struct {
	t *TCPGlobalInfo // a pointer to the tcp global state struct

	socketId       int32
	port           uint16
	pendingSocketC chan struct {
		*proto.TCPPacket
		netip.Addr
	} // since we need info about TCP header and the srcIP
}

type VTCPConn struct { // represents a TCP socket
	t *TCPGlobalInfo // a pointer to the tcp global state struct

	TCPEndpointID
	socketId int32

	state   State
	stateMu sync.RWMutex // for protecting access to state

	iss            uint32         // initial send sequence number (unsafe - should stay unchanged once initialized)
	irs            uint32         // initial receive sequence number (unsafe - should stay unchanged once connection established)
	sndNxt         *atomic.Uint32 // SND.NXT - next seq num to use for sending
	sndUna         *atomic.Uint32 // SND.UNA - the largest ACK we received
	sndWnd         *atomic.Int32  // SND.WND - the other side's window size
	expectedSeqNum *atomic.Uint32 // RCV.NXT- the ACK num we should send to the other side
	windowSize     *atomic.Int32  // RCV.WND - range 0 ~ 65535
	mu             sync.RWMutex   // for protecting access to send/recv related fields

	sendBuf       *sendBuf
	recvBuf       *recvBuf
	earlyArrivalQ PriorityQueue

	recvChan      chan *proto.TCPPacket // for receiving tcp packets dispatched to this connection
	timeWaitReset chan bool

	inflightQ    *deque.Deque[*packetMetadata] // retransmission queue
	inflightMu   sync.RWMutex
	rtoMu        sync.RWMutex // for protecting access to retransmission related fields
	retransTimer *time.Timer
	rto          float64      // Retransmission Timeout
	firstRTT     *atomic.Bool // if this is the first measurement of RTO
	sRTT         float64      // Smooth Round Trip Time
	rtoIsRunning bool         // if RTO is running (6298 - 5.1)
}

func Init(ip *ipstack.IPGlobalInfo) (*TCPGlobalInfo, error) {
	// extract local addr
	var localAddr netip.Addr
	for _, i := range ip.Interfaces {
		localAddr = i.AssignedIP
		break
	}
	if !localAddr.IsValid() {
		return nil, errors.New("invalid local ip address")
	}

	t := &TCPGlobalInfo{
		IP:            ip,
		localAddr:     localAddr,
		listenerTable: make(map[uint16]*VTCPListener),
		connTable:     make(map[TCPEndpointID]*VTCPConn),
		tableMu:       sync.RWMutex{},
		socketNum:     -1,
	}
	ip.RegisterRecvHandler(proto.ProtoNumTCP, tcpRecvHandler(t))

	return t, nil
}

/************************************ TCP API ***********************************/

// (Passive OPEN in CLOSED state) VListen creates a new listening socket bound to the specified port
func VListen(t *TCPGlobalInfo, port uint16) (*VTCPListener, error) {
	// check the listener table to see if the port is already in use
	if t.isPortInUse(port) {
		return nil, fmt.Errorf("port %v already in use", port)
	}
	// create a new listener socket
	l := NewListenerSocket(t, port)
	t.bindListener(port, l)
	fmt.Printf("Created a listener socket with id %v\n", l.socketId)
	return l, nil
}

// (Active OPEN in CLOSED state)
func VConnect(t *TCPGlobalInfo, addr netip.Addr, port uint16) (*VTCPConn, error) {
	// create a new endpoint ID with a random port number
	endpoint := TCPEndpointID{
		LocalAddr:  t.localAddr,
		LocalPort:  generateRandomPortNum(t),
		RemoteAddr: addr,
		RemotePort: port,
	}
	if t.socketExists(endpoint) {
		return nil, fmt.Errorf("socket already exsists with local vip %v port %v and remote vip %v port %v", endpoint.LocalAddr, endpoint.LocalPort, endpoint.RemoteAddr, endpoint.RemotePort)
	}
	// create the socket
	conn := NewSocket(t, SYN_SENT, endpoint, 0)
	t.bindSocket(endpoint, conn)

	// handshake & possible retransmissions
	conn.sndNxt.Add(1)
	for i := 0; i <= MAX_RETRANSMISSIONS; i++ {
		suc := conn.handshakeRetrans(i, true)
		if suc {
			fmt.Printf("Created a new socket with id %v\n", conn.socketId)
			go conn.run() // conn goes into ESTABLISHED state
			return conn, nil
		}
	}
	t.deleteSocket(endpoint)
	return nil, fmt.Errorf("error establishing connection from %v to %v", netip.AddrPortFrom(endpoint.LocalAddr, endpoint.LocalPort), netip.AddrPortFrom(addr, port))
}

/************************************ TCP Handler ***********************************/

func tcpRecvHandler(t *TCPGlobalInfo) func(*proto.IPPacket) {
	return func(ipPacket *proto.IPPacket) {
		// unmarshal and validate the packet
		tcpPacket := new(proto.TCPPacket)
		tcpPacket.Unmarshal(ipPacket.Payload[:ipPacket.Header.TotalLen-ipPacket.Header.Len])

		if !proto.ValidTCPChecksum(tcpPacket, ipPacket.Header.Src, ipPacket.Header.Dst) {
			logger.Printf("packet dropped because checksum validation failed\n")
			return
		}
		if t.localAddr != ipPacket.Header.Dst {
			logger.Printf("packet dropped because the packet with dst %v is not for %v.\n", ipPacket.Header.Dst, t.localAddr)
			return
		}

		// find and forward the packet to corresponding socket
		endpoint := TCPEndpointID{
			LocalAddr:  ipPacket.Header.Dst,
			LocalPort:  tcpPacket.TcpHeader.DstPort,
			RemoteAddr: ipPacket.Header.Src,
			RemotePort: tcpPacket.TcpHeader.SrcPort,
		}
		t.tableMu.RLock()
		defer t.tableMu.RUnlock()

		if s, ok := t.connTable[endpoint]; !ok {
			l, ok := t.listenerTable[endpoint.LocalPort]
			if !ok {
				logger.Printf("packet dropped because there's no matching listener and normal sockets for port %v.\n", endpoint.LocalPort)
				return
			}
			l.pendingSocketC <- struct {
				*proto.TCPPacket
				netip.Addr
			}{tcpPacket, endpoint.RemoteAddr}
		} else {
			s.recvChan <- tcpPacket
		}
	}
}
