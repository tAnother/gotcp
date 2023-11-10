package tcpstack

import (
	"iptcp-nora-yu/pkg/proto"

	"github.com/google/netstack/tcpip/header"
)

type State int

const (
	CLOSED State = iota
	LISTEN
	SYN_RECEIVED
	SYN_SENT
	ESTABLISHED
	FIN_WAIT_1
	FIN_WAIT_2
	CLOSING
	TIME_WAIT
	CLOSE_WAIT
	LAST_ACK
)

var stateString = []string{
	"CLOSED",
	"LISTEN",
	"SYN_RECEIVED",
	"SYN_SENT",
	"ESTABLISHED",
	"FIN_WAIT_1",
	"FIN_WAIT_2",
	"CLOSING",
	"TIME_WAIT",
	"CLOSE_WAIT",
	"LAST_ACK",
}

var stateFuncMap = []func(*VTCPConn){
	nil,
	nil,
	handleSynRcvd,
	handleSynSent,
	handleEstablished,
	handleFinWait1,
	handleFinWait2,
	handleClosing,
	handleTimeWait,
	handleCloseWait,
	handleLastAck,
}

func handleSynRcvd(conn *VTCPConn) {
	// wait for ACK, or close signal
	select {
	case <-conn.closeC:
		// transit to active close state?

	case packet := <-conn.recvChan:
		// if we have an only-ACK packet
		if packet.TcpHeader.Flags != header.TCPFlagAck {
			// TODO: should we delete the socket from the table????
			conn.t.deleteSocket(conn.TCPEndpointID)
			logger.Printf("error accpeting new conn. Flag of the back packet is not only-ACK but %v\n", packet.TcpHeader.Flags)
			return
		}

		// check if seqNum matches our expected SeqNum
		if packet.TcpHeader.SeqNum != conn.expectedSeqNum.Load() {
			// should we delete the socket from the table????
			conn.t.deleteSocket(conn.TCPEndpointID)
			logger.Printf("error accpeting new conn. Seq number received %v, expected %v\n", packet.TcpHeader.SeqNum, conn.expectedSeqNum)
			return
		}
		if packet.TcpHeader.AckNum != conn.localInitSeqNum+1 {
			// should we delete the socket from the table????
			conn.t.deleteSocket(conn.TCPEndpointID)
			logger.Printf("error accpeting new conn. Ack number received %v, expected %v\n", packet.TcpHeader.AckNum, conn.localInitSeqNum+1)
			return
		}

		conn.largestAck.Store(packet.TcpHeader.AckNum)
		conn.sendBuf.mu.Lock()
		conn.sendBuf.wnd = packet.TcpHeader.WindowSize
		conn.sendBuf.mu.Unlock()

		conn.stateMu.Lock()
		conn.state = ESTABLISHED
		conn.stateMu.Unlock()
		go conn.send()
	}
}

func handleSynSent(conn *VTCPConn) {
	// wait for SYN+ACK, or close signal
	select {
	case <-conn.closeC:
		// transit to active close state?
	case packet := <-conn.recvChan:
		if packet.TcpHeader.Flags != header.TCPFlagSyn|header.TCPFlagAck {
			conn.t.deleteSocket(conn.TCPEndpointID)
			logger.Printf("Flag is not SYN+ACK but %v\n", packet.TcpHeader.Flags)
			return
		}
		if packet.TcpHeader.AckNum != conn.localInitSeqNum+1 {
			conn.t.deleteSocket(conn.TCPEndpointID)
			logger.Printf("error completing a handshake. Ack number received %v, expected %v\n", packet.TcpHeader.AckNum, conn.localInitSeqNum+1)
			return
		}

		conn.largestAck.Store(packet.TcpHeader.AckNum)
		conn.remoteInitSeqNum = packet.TcpHeader.SeqNum
		conn.expectedSeqNum.Store(packet.TcpHeader.SeqNum + 1)

		// send ACK
		newTcpPacket := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
			conn.seqNum.Load(), packet.TcpHeader.SeqNum+1,
			header.TCPFlagAck, make([]byte, 0), BUFFER_CAPACITY)
		err := send(conn.t, newTcpPacket, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
		if err != nil {
			// should we delete the socket from the table????
			conn.t.deleteSocket(conn.TCPEndpointID)
			return
		}

		conn.stateMu.Lock()
		conn.state = ESTABLISHED
		conn.stateMu.Unlock()
		go conn.send()
	}

}

func handleEstablished(conn *VTCPConn) {
	select {
	case <-conn.closeC:
		// transit to active close state?

	case packet := <-conn.recvChan:
		payloadSize := len(packet.Payload)

		if packet.TcpHeader.AckNum > conn.largestAck.Load() {
			conn.largestAck.Store(packet.TcpHeader.AckNum)
			conn.sendBuf.ack(packet.TcpHeader.AckNum)
		}

		if payloadSize > 0 {
			if payloadSize > int(conn.recvBuf.FreeSpace()) {
				logger.Printf("packet is outside the window, dropping the packet")
				return
			}

			if payloadSize > int(conn.recvBuf.NextExpectedByte()) {
				// TODO : Out-of-order: queue as early arrival or should we just write to the buffer and let the buffer handle it
			}

			// at this point, we received the next byte expected segment,
			// we should deliver its next segment if it was a early arrival  --> check early arrival queue
			// TODO : Out-of-order
			n, err := conn.recvBuf.Write(packet.Payload)
			if err != nil {
				logger.Printf("failed to write to the read buffer error: %v. dropping the packet...", err)
				//should we drop the packet...?
				return
			}
			logger.Printf("received %d bytes\n", n)

			conn.expectedSeqNum.Add(uint32(n)) //TODO : Out-of-order should be updated for early arrival; should not update expected seq num here
			newWindowSize := conn.recvBuf.FreeSpace()
			conn.windowSize.Store(int32(min(BUFFER_CAPACITY, newWindowSize)))

			// sends largest contiguous ack and left-over window size back
			newTcpPacket := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
				conn.seqNum.Load(), conn.expectedSeqNum.Load(),
				header.TCPFlagAck, make([]byte, 0), newWindowSize)
			err = send(conn.t, newTcpPacket, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
			if err != nil {
				return
			}
		}

		if packet.TcpHeader.Flags == header.TCPFlagFin|header.TCPFlagAck {
			//send ack back
			newTcpPacket := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
				packet.TcpHeader.AckNum, packet.TcpHeader.SeqNum+1,
				header.TCPFlagAck, make([]byte, 0), BUFFER_CAPACITY) // TODO: seqnum and acknum correct?

			err := send(conn.t, newTcpPacket, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
			if err != nil {
				logger.Println(err)
				return
			}
			conn.seqNum.Add(1)
			conn.stateMu.Lock()
			conn.state = CLOSE_WAIT
			conn.stateMu.Unlock()
			return
		}
	}
}

func handleCloseWait(conn *VTCPConn) {
	conn.state = LAST_ACK
}

func handleLastAck(conn *VTCPConn) {
	conn.state = CLOSED
}

func handleFinWait1(conn *VTCPConn) {
	// packet := <-conn.recvChan
}

func handleFinWait2(conn *VTCPConn) {
	// packet := <-conn.recvChan
	conn.state = TIME_WAIT
}

func handleClosing(conn *VTCPConn) {
	// packet := <-conn.recvChan
	conn.state = TIME_WAIT
}

func handleTimeWait(conn *VTCPConn) {
	conn.state = CLOSED
}

// func (conn *VTCPConn) handleClosed() error {
// 	if conn == nil {
// 		return fmt.Errorf("connection does not exist")
// 	}
// 	return nil
// }

// func (conn *VTCPConn) handleCloseWait() error {
// 	return io.EOF
// }

// func (conn *VTCPConn) handleTimeWait() error {
// 	return fmt.Errorf("connection closing")
// }
