package tcpstack

import (
	"fmt"
	"iptcp-nora-yu/pkg/proto"
	"net/netip"
	"sync/atomic"

	"github.com/google/netstack/tcpip/header"
)

// Creates and binds the listener socket
func NewListenerSocket(port uint16, addr netip.Addr) *VTCPListener {
	listener := &VTCPListener{
		socketId: atomic.AddInt32(&socketId, 1),
		addr:     addr,
		port:     port,
		pendingSocket: make(chan struct {
			*proto.TCPPacket
			netip.Addr
		}),
	}
	return listener
}

// Three-way handshake for receiver side happens here.
func (l *VTCPListener) VAccept() (*VTCPConn, error) {
	select {
	case pair := <-l.pendingSocket: //the incoming TCP packet should be valid: matching process is done.
		srcIP := pair.Addr
		tcpPacket := pair.TCPPacket

		// 1. check if it's an only-SYN packet
		if tcpPacket.TcpHeader.Flags != header.TCPFlagSyn {
			return nil, fmt.Errorf("error accpeting new conn. Flag is not only-SYN but %v", tcpPacket.TcpHeader.Flags)
		}

		// 2. create a new normal socket. state should be SYN_RECV at this point
		endpoint := TCPEndpointID{
			localAddr:  l.addr,
			localPort:  l.port,
			remoteAddr: srcIP,
			remotePort: tcpPacket.TcpHeader.SrcPort,
		}
		if socketExists(endpoint) {
			return nil, fmt.Errorf("socket already exsists with local vip %v port %v and remote vip %v port %v", endpoint.localAddr, endpoint.localPort, endpoint.remoteAddr, endpoint.remotePort)
		}
		conn := NewSocket(SYN_RECEIVED, endpoint, tcpPacket.TcpHeader.SeqNum)
		bindSocket(endpoint, conn)

		// 3. send back SYN+ACK packet
		newTcpPacket := proto.NewTCPacket(endpoint.localPort, endpoint.remotePort,
			conn.localInitSeqNum, tcpPacket.TcpHeader.SeqNum+1,
			header.TCPFlagSyn|header.TCPFlagAck, make([]byte, 0))
		conn.expectedSeqNum = conn.remoteInitSeqNum + 1
		err := tcb.Driver.SendTcpPacket(newTcpPacket, endpoint.localAddr, endpoint.remoteAddr)
		if err != nil {
			//TODO: should we delete the socket from the table????
			deleteSocket(endpoint)
			return nil, fmt.Errorf("error sending SYN+ACK packet back to %v", conn)
		}

		//4. state machine: syn_recvd --> established
		err = handleSynRecvd(conn, endpoint)
		if err != nil {
			return nil, err
		}

		//5. connection established.
		go conn.handleConnection()
		fmt.Printf("New connection on socket %v => created new socket %v\n", l.socketId, conn.socketId)
		return conn, nil
	}

}

// func (l *VTCPListener) VClose() error // not for milestone I
