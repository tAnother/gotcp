package tcpstack

import (
	"fmt"
	"iptcp-nora-yu/pkg/proto"
	"net/netip"

	"github.com/google/netstack/tcpip/header"
)

const (
	CLOSED       State = "CLOSED"
	LISTEN       State = "LISTEN"
	SYN_RECEIVED State = "SYN_RECEIVED"
	SYN_SENT     State = "SYN_SENT"
	CLOSING      State = "CLOSING"
	FIN_WAIT_1   State = "FIN_WAIT_1"
	FIN_WAIT_2   State = "FIN_WAIT_2"
	TIME_WAIT    State = "TIME_WAIT"
	CLOSE_WAIT   State = "CLOSE_WAIT"
	LAST_ACK     State = "LAST_ACK"
	ESTABLISHED  State = "ESTABLISHED"
)

func handleSynRecvd(conn *VTCPConn, endPoint TCPEndpointID) error {
	// 1. If we have an only-ACK packet back
	back := <-conn.recvChan
	if back.TcpHeader.Flags != header.TCPFlagAck {
		//TODO: should we delete the socket from the table????
		deleteSocket(endPoint)
		return fmt.Errorf("error accpeting new conn. Flag of the back packet is not only-ACK but %v", back.TcpHeader.Flags)
	}

	// 2. Check if seqNum matches our expected SeqNum
	if back.TcpHeader.SeqNum != conn.expectedSeqNum {
		//TODO: should we delete the socket from the table????
		deleteSocket(endPoint)
		return fmt.Errorf("error accpeting new conn. Seq number received %v, expected %v", back.TcpHeader.SeqNum, conn.expectedSeqNum)
	}
	if back.TcpHeader.AckNum != conn.localInitSeqNum+1 {
		//TODO: should we delete the socket from the table????
		deleteSocket(endPoint)
		return fmt.Errorf("error accpeting new conn. Ack number received %v, expected %v", back.TcpHeader.AckNum, conn.localInitSeqNum+1)
	}

	// 3. If does, conn established. Three-way handshake completed.
	conn.setState(ESTABLISHED)
	return nil
}

func handleSynSent(conn *VTCPConn, endpoint TCPEndpointID) error {
	select {
	case tcpPacket := <-conn.recvChan:
		//6. check ack, seq number and flags
		if tcpPacket.TcpHeader.Flags != header.TCPFlagSyn|header.TCPFlagAck {
			deleteSocket(endpoint)
			return fmt.Errorf("error completing a handshake. Flag is not SYN+ACK but %v", tcpPacket.TcpHeader.Flags)
		}
		if tcpPacket.TcpHeader.AckNum != conn.localInitSeqNum+1 {
			deleteSocket(endpoint)
			return fmt.Errorf("error completing a handshake. Ack number received %v, expected %v", tcpPacket.TcpHeader.AckNum, conn.localInitSeqNum+1)
		}
		//should do something to the socket seq number fields here....
		//7. send ack tcp packet
		newTcpPacket := proto.NewTCPacket(endpoint.localPort, endpoint.remotePort,
			tcpPacket.TcpHeader.AckNum, tcpPacket.TcpHeader.SeqNum+1,
			header.TCPFlagAck, make([]byte, 0))
		conn.expectedSeqNum = conn.remoteInitSeqNum + 1
		err := tcb.Driver.SendTcpPacket(newTcpPacket, endpoint.localAddr, endpoint.remoteAddr)
		if err != nil {
			//TODO: should we delete the socket from the table????
			deleteSocket(endpoint)
			return fmt.Errorf("error sending ACK packet from %v to %v", netip.AddrPortFrom(endpoint.localAddr, endpoint.localPort), netip.AddrPortFrom(endpoint.remoteAddr, endpoint.remotePort))
		}
		//8. established...
		conn.setState(ESTABLISHED)
		return nil
	}
}
