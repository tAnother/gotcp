package tcpstack

import (
	"fmt"
	"iptcp-nora-yu/pkg/proto"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/google/netstack/tcpip/header"
)

// Creates and binds the listener socket
func NewListenerSocket(t *TCPGlobalInfo, port uint16) *VTCPListener {
	listener := &VTCPListener{
		t:        t,
		socketId: atomic.AddInt32(&t.socketNum, 1),
		port:     port,
		pendingSocketC: make(chan struct {
			*proto.TCPPacket
			netip.Addr
		}),
	}
	return listener
}

// The LISTEN state. Three-way handshake for receiver side happens here.
func (l *VTCPListener) VAccept() (*VTCPConn, error) {
	pair := <-l.pendingSocketC
	srcIP := pair.Addr
	tcpPacket := pair.TCPPacket

	// 1. check if it's an only-SYN packet
	if tcpPacket.TcpHeader.Flags != header.TCPFlagSyn {
		return nil, fmt.Errorf("error accpeting new conn. Flag is not only-SYN but %v", tcpPacket.TcpHeader.Flags)
	}

	// 2. create a new normal socket. state should be SYN_RECV at this point
	endpoint := TCPEndpointID{
		LocalAddr:  l.t.localAddr,
		LocalPort:  l.port,
		RemoteAddr: srcIP,
		RemotePort: tcpPacket.TcpHeader.SrcPort,
	}
	if l.t.socketExists(endpoint) {
		return nil, fmt.Errorf("socket already exsists with local vip %v port %v and remote vip %v port %v", endpoint.LocalAddr, endpoint.LocalPort, endpoint.RemoteAddr, endpoint.RemotePort)
	}
	conn := NewSocket(l.t, SYN_RECEIVED, endpoint, tcpPacket.TcpHeader.SeqNum)
	l.t.bindSocket(endpoint, conn)

	// 3. send back SYN+ACK packet
	packet, err := conn.sendCTL(conn.iss, conn.expectedSeqNum.Load(), header.TCPFlagSyn|header.TCPFlagAck)
	if err != nil {
		l.t.deleteSocket(endpoint)
		return nil, fmt.Errorf("error sending SYN+ACK packet back to %v", conn)
	}

	// TODO : retransmission. This should block
	conn.inflightQ.PushBack(&packetMetadata{length: 0, packet: packet, timeSent: time.Now()}) //  no need to lock the queue?
	conn.startOrResetRetransTimer(false)
	conn.handleRTO() // TODO: this will block? but should return on success

	fmt.Printf("New connection on socket %v => created new socket %v\n", l.socketId, conn.socketId)

	conn.sndNxt.Add(1)
	go conn.run() // conn goes into SYN_RECEIVED state
	return conn, nil
}

func (l *VTCPListener) VClose() error {
	l.t.tableMu.Lock()
	defer l.t.tableMu.Unlock()
	if _, ok := l.t.listenerTable[l.port]; !ok {
		return fmt.Errorf("listener with port %v already closed", l.port)
	}
	delete(l.t.listenerTable, l.port)
	return nil
}
