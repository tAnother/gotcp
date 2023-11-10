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
		conn.sendBuf.wnd = int(packet.TcpHeader.WindowSize)
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

	case segment := <-conn.recvChan:
		segLen := len(segment.Payload)

		if segment.TcpHeader.AckNum > conn.largestAck.Load() {
			conn.largestAck.Store(segment.TcpHeader.AckNum)
			conn.sendBuf.ack(segment.TcpHeader.AckNum)
		}

		if segLen > 0 {
			if conn.recvBuf.IsFull() {
				logger.Printf("window size is 0. The segment is not acceptable. Start zero window probing...")

				if segment.TcpHeader.Flags&header.TCPFlagRst == header.TCPFlagRst { // but if RST bit is set, drop the packet and return
					logger.Printf("a retransmitted non-accpetable packet. Dropping the packet...")
					return
				}
				//TODO: zero window probing. Sends 1-byte any data back until advertised window size > 0
				// seq = sendBuff.nxt; ack = recvBuff.nxt; flag = ack
				logger.Printf("Ack is sent. dropping the packet...")
				return
			}

			if segment.TcpHeader.SeqNum > conn.recvBuf.NextExpectedByte() { // TODO : maybe needs to change the range of head tail of buff
				// TODO : Out-of-order: queue as early arrival or should we just write to the buffer and let the buffer handle it
				if segment.TcpHeader.Flags&header.TCPFlagRst == header.TCPFlagRst { // send challenge ACK
					logger.Printf("challenge ACK is sent. Dropping the packet...")
					// seq = sendBuff.nxt; ack = recvBuff.nxt; flag = ack
					return
				}
				return
			}

			// at this point, we received the next byte expected segment, and it's inside the window
			if segment.TcpHeader.Flags&header.TCPFlagRst == header.TCPFlagRst {
				// reset the connection RFC9293 3.10.7
				return
			}

			// TODO : Out-of-order : we should deliver its next segment if it was a early arrival  --> check early arrival queue
			n, err := conn.recvBuf.Write(segment.Payload) // this will write as much as possible. Trim off any additional data
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
				header.TCPFlagAck, make([]byte, 0), uint16(newWindowSize))
			err = send(conn.t, newTcpPacket, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
			if err != nil {
				return
			}
		}

		if segment.TcpHeader.Flags == header.TCPFlagFin|header.TCPFlagAck {
			//send ack back
			newTcpPacket := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
				segment.TcpHeader.AckNum, segment.TcpHeader.SeqNum+1,
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
	// segment := <-conn.recvChan
	//check RST bit
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
	// segment := <-conn.recvChan
	//check RST bit
	//check SYN bit
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
