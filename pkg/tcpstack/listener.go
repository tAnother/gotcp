package tcpstack

import (
	"fmt"
	"iptcp-nora-yu/pkg/proto"
	"net/netip"

	"github.com/google/netstack/tcpip/header"
)

// Creates and binds the listener socket
func NewListenerSocket(t *TCPGlobalInfo, port uint16) *VTCPListener {
	listener := &VTCPListener{
		t:        t,
		socketId: t.socketNum.Add(1),
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

	// handshake & possible retransmissions
	suc := conn.doHandshakes(0, false)
	if suc {
		fmt.Printf("New connection on socket %v => created new socket %v\n", l.socketId, conn.socketId)
		go conn.run() // conn goes into ESTABLISHED state
		return conn, nil
	}
	l.t.deleteSocket(endpoint)
	return nil, fmt.Errorf("error establishing connection for %v", netip.AddrPortFrom(endpoint.LocalAddr, endpoint.LocalPort))
}

func (l *VTCPListener) VClose() error {
	l.t.tableMu.Lock()
	if _, ok := l.t.listenerTable[l.port]; !ok {
		l.t.tableMu.Unlock()
		return fmt.Errorf("listener with port %v already closed", l.port)
	}
	l.t.tableMu.Unlock()
	l.t.deleteListener(l.port)
	return nil
}
