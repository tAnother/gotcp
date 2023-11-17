package tcpstack

import (
	"fmt"
	"iptcp-nora-yu/pkg/proto"
	"time"

	"github.com/google/netstack/tcpip/header"
)

// TODO : For each state, need to refactor and add handleSeq, handleSyn.

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

var stateFuncMap = []func(*VTCPConn, *proto.TCPPacket){
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

func (conn *VTCPConn) stateMachine(segment *proto.TCPPacket) {
	conn.stateMu.RLock()
	stateFunc := stateFuncMap[conn.state]
	conn.stateMu.RUnlock()
	if stateFunc != nil {
		stateFunc(conn, segment)
	}
}

func handleSynRcvd(conn *VTCPConn, packet *proto.TCPPacket) {
	// if we have an only-ACK packet
	if packet.TcpHeader.Flags != header.TCPFlagAck {
		// TODO: should we delete the socket from the table????
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Printf("error accpeting new conn. Flag of the back packet is not only-ACK but %v\n", packet.TcpHeader.Flags)
		return
	}

	// check if seqNum matches our expected SeqNum
	// TODO : out-of-order
	if packet.TcpHeader.SeqNum != conn.expectedSeqNum.Load() {
		// should we delete the socket from the table????
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Printf("error accpeting new conn. Seq number received %v, expected %v\n", packet.TcpHeader.SeqNum, conn.expectedSeqNum)
		return
	}
	if packet.TcpHeader.AckNum != conn.iss+1 {
		// should we delete the socket from the table????
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Printf("error accpeting new conn. Ack number received %v, expected %v\n", packet.TcpHeader.AckNum, conn.iss+1)
		return
	}

	if packet.IsFin() {
		err := handleFin(packet, conn)
		if err != nil {
			logger.Printf("error handling fin packet")
			return
		}
		conn.stateMu.Lock()
		conn.state = CLOSE_WAIT
		conn.stateMu.Unlock()
		return
	}

	conn.sndUna.Store(packet.TcpHeader.AckNum)
	conn.sndWnd.Store(int32(packet.TcpHeader.WindowSize))

	conn.stateMu.Lock()
	conn.state = ESTABLISHED
	conn.stateMu.Unlock()
	go conn.send()
	// }
}

func handleSynSent(conn *VTCPConn, packet *proto.TCPPacket) {
	if packet.TcpHeader.Flags != header.TCPFlagSyn|header.TCPFlagAck {
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Printf("Flag is not SYN+ACK but %v\n", packet.TcpHeader.Flags)
		return
	}
	if packet.TcpHeader.AckNum != conn.iss+1 {
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Printf("error completing a handshake. Ack number received %v, expected %v\n", packet.TcpHeader.AckNum, conn.iss+1)
		return
	}

	conn.sndUna.Store(packet.TcpHeader.AckNum)
	conn.sndWnd.Store(int32(packet.TcpHeader.WindowSize))
	conn.irs = packet.TcpHeader.SeqNum
	conn.expectedSeqNum.Store(packet.TcpHeader.SeqNum + 1)

	// send ACK
	newTcpPacket := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
		conn.sndNxt.Load(), conn.expectedSeqNum.Load(),
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

// }

func handleEstablished(conn *VTCPConn, segment *proto.TCPPacket) {
	segLen := len(segment.Payload)

	// check ACK
	if !segment.IsAck() {
		logger.Printf("ACK bit is off. Dropping the packet...")
	} else {
		err := handleAck(segment, conn)
		if err != nil {
			logger.Println(err)
			return
		}
	}

	if segLen > 0 {
		if conn.recvBuf.IsFull() {
			logger.Printf("window size is 0. The segment is not acceptable.")

			if segment.TcpHeader.Flags&header.TCPFlagRst == header.TCPFlagRst { // but if RST bit is set, drop the packet and return
				logger.Printf("a retransmitted non-accpetable packet. Dropping the packet...")
				return
			}
			//TODO: zero window probing. Sends 1-byte any data back until advertised window size > 0
			// seq = sendBuff.nxt; ack = recvBuff.nxt; flag = ack
			logger.Printf("Ack is sent. Dropping the packet...")
			return
		}

		if segment.TcpHeader.SeqNum > conn.expectedSeqNum.Load() { // TODO : maybe needs to change the range of head tail of buff
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
			conn.sndNxt.Load(), conn.expectedSeqNum.Load(),
			header.TCPFlagAck, make([]byte, 0), uint16(newWindowSize))
		err = send(conn.t, newTcpPacket, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
		if err != nil {
			return
		}
	}

	if segment.IsFin() {
		err := handleFin(segment, conn)
		if err != nil {
			logger.Printf("error handling fin packet")
			return
		}
		conn.stateMu.Lock()
		conn.state = CLOSE_WAIT
		conn.stateMu.Unlock()
		return
	}
}

func handleFinWait1(conn *VTCPConn, segment *proto.TCPPacket) {
	conn.stateMu.Lock()
	defer conn.stateMu.Unlock()
	//1. preprocess ack
	if !segment.IsAck() {
		logger.Printf("ACK bit is off. Dropping the packet...")
		return
	} else {
		err := handleAck(segment, conn)
		if err != nil {
			logger.Println(err)
			return
		}
		//2. check if fin is acked
		if segment.TcpHeader.AckNum == conn.sndNxt.Load() {
			conn.state = FIN_WAIT_2
			return
		}
	}

	//3. check fin bit
	if segment.IsFin() {
		if conn.sndNxt.Load() == segment.TcpHeader.AckNum {
			conn.state = TIME_WAIT
			go timeWaitTimer(conn)
		} else {
			conn.state = CLOSING
		}
	}
}

func handleFinWait2(conn *VTCPConn, segment *proto.TCPPacket) {
	conn.stateMu.Lock()
	defer conn.stateMu.Unlock()
	//1. preprocess ack
	if !segment.IsAck() {
		logger.Printf("ACK bit is off. Dropping the packet...")
		return
	} else {
		err := handleAck(segment, conn)
		if err != nil {
			logger.Println(err)
			return
		}

		// ack CLOSE
		if conn.inflightQ.Len() == 0 {
			conn.expectedSeqNum.Store(segment.TcpHeader.SeqNum + 1)
			packet := proto.NewTCPacket(conn.LocalPort, conn.RemotePort, conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck, make([]byte, 0), uint16(conn.windowSize.Load()))
			err = send(conn.t, packet, conn.LocalAddr, conn.RemoteAddr)
			if err != nil {
				logger.Println(err)
				return
			}
		}
	}
	if segment.IsFin() {
		conn.state = TIME_WAIT
		go timeWaitTimer(conn)
	}
}

func handleCloseWait(conn *VTCPConn, segment *proto.TCPPacket) {

	//1. preprocess ack
	if !segment.IsAck() {
		logger.Printf("ACK bit is off. Dropping the packet...")
	} else {
		err := handleAck(segment, conn)
		if err != nil {
			logger.Println(err)
			return
		}
	}

	// if FIN is set, no-op
}

func handleClosing(conn *VTCPConn, segment *proto.TCPPacket) {
	conn.stateMu.Lock()
	defer conn.stateMu.Unlock()
	//1. preprocess ack
	if !segment.IsAck() {
		logger.Printf("ACK bit is off. Dropping the packet...")
	} else {
		err := handleAck(segment, conn)
		if err != nil {
			logger.Println(err)
			return
		}
		if conn.sndNxt.Load() == segment.TcpHeader.AckNum {
			conn.state = TIME_WAIT
		} else {
			logger.Printf("Our FIN is not ACKed. Dropping the packet...")
		}
	}

	// if FIN is set, no-op
}

func handleLastAck(conn *VTCPConn, segment *proto.TCPPacket) {
	conn.stateMu.Lock()
	defer conn.stateMu.Unlock()
	//1. preprocess ack
	if !segment.IsAck() {
		logger.Printf("ACK bit is off. Dropping the packet...")
	} else {
		err := handleAck(segment, conn)
		if err != nil {
			logger.Println(err)
			return
		}
		if conn.sndNxt.Load() == segment.TcpHeader.AckNum {
			conn.state = CLOSED
			conn.t.deleteSocket(conn.TCPEndpointID)
		} else {
			logger.Printf("Our FIN is not ACKed. Dropping the packet...")
		}
	}

	// if FIN is set, no-op
}

func handleTimeWait(conn *VTCPConn, segment *proto.TCPPacket) {
	conn.stateMu.Lock()
	defer conn.stateMu.Unlock()

	//check SYN bit

	if !segment.IsAck() {
		logger.Printf("ACK bit is off. Dropping the packet...")
	} else {
		err := handleAck(segment, conn)
		if err != nil {
			logger.Println(err)
			return
		}
		conn.expectedSeqNum.Store(segment.TcpHeader.SeqNum + 1)
		packet := proto.NewTCPacket(conn.LocalPort, conn.RemotePort, conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck, make([]byte, 0), uint16(conn.windowSize.Load()))
		err = send(conn.t, packet, conn.LocalAddr, conn.RemoteAddr)
		if err != nil {
			logger.Println(err)
			return
		}
		conn.timeWaitReset <- true
	}

	// if FIN is set, restart timer
	if segment.IsFin() {
		conn.timeWaitReset <- true
	}

}

// Timer for TIME-WAIT
// TODO : Not sure if the reset works...
func timeWaitTimer(conn *VTCPConn) {
	timer := time.NewTimer(2 * MSL)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			// expires
			conn.stateMu.Lock()
			conn.state = CLOSED
			conn.stateMu.Unlock()
			conn.t.deleteSocket(TCPEndpointID{LocalAddr: conn.LocalAddr, RemoteAddr: conn.RemoteAddr, LocalPort: conn.LocalPort, RemotePort: conn.RemotePort})
			return
		case <-conn.timeWaitReset:
			if !timer.Stop() {
				<-timer.C // Drain the channel if the timer already expired
			}
			timer = time.NewTimer(2 * MSL)
		}
	}
}

// See RFC 9293 - 3.10.4 CLOSE Call
func (conn *VTCPConn) activeClose() (err error) {
	conn.stateMu.Lock()
	defer conn.stateMu.Unlock()
	//at this point, closed state and listen state are already handled
	switch conn.state {
	case SYN_SENT:
		err = fmt.Errorf("connection closing")
		conn.t.deleteSocket(conn.TCPEndpointID)
	case SYN_RECEIVED:
		conn.sendBuf.mu.Lock()
		size := conn.numBytesNotSent()
		conn.sendBuf.mu.Unlock()
		// If no SENDs have been issued and there is no pending data to send
		if conn.sndNxt.Load() != conn.iss+1 && size == 0 {
			finPacket := proto.NewTCPacket(conn.LocalPort, conn.RemotePort, conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagFin, make([]byte, 0), uint16(conn.windowSize.Load()))
			err = send(conn.t, finPacket, conn.LocalAddr, conn.RemoteAddr)
			if err != nil {
				return err
			}
			conn.state = FIN_WAIT_1
		}
	case ESTABLISHED:
		//  wait until send buff is empty. Potential Deadlock
		conn.sendBuf.mu.Lock()
		size := conn.numBytesNotSent()
		conn.sendBuf.mu.Unlock()

		for size > 0 {
			conn.sendBuf.mu.Lock()
			size = conn.numBytesNotSent()
			conn.sendBuf.mu.Unlock()
		}

		finPacket := proto.NewTCPacket(conn.LocalPort, conn.RemotePort, conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagFin|header.TCPFlagAck, make([]byte, 0), uint16(conn.windowSize.Load()))
		err = send(conn.t, finPacket, conn.LocalAddr, conn.RemoteAddr)
		if err != nil {
			return err
		}
		conn.sndNxt.Add(1)
		conn.state = FIN_WAIT_1
	case CLOSE_WAIT:
		//  wait until send buff is empty. Potential Deadlock
		conn.sendBuf.mu.Lock()
		size := conn.numBytesNotSent()
		conn.sendBuf.mu.Unlock()

		for size > 0 {
			conn.sendBuf.mu.Lock()
			size = conn.numBytesNotSent()
			conn.sendBuf.mu.Unlock()
		}

		finPacket := proto.NewTCPacket(conn.LocalPort, conn.RemotePort, conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagFin|header.TCPFlagAck, make([]byte, 0), uint16(conn.windowSize.Load()))
		err = send(conn.t, finPacket, conn.LocalAddr, conn.RemoteAddr)
		if err != nil {
			return err
		}
		conn.sndNxt.Add(1)
		conn.state = LAST_ACK
	default:
		err = fmt.Errorf("connection closing")

	}
	return err
}

/************************************ Handle different bits of the Segment ***********************************/

// Preprocess ACK bit
func handleAck(segment *proto.TCPPacket, conn *VTCPConn) (err error) {
	segAck := segment.TcpHeader.AckNum
	if conn.sndUna.Load()-uint32(conn.sendBufFreeSpace()) > segAck || conn.sndNxt.Load() < segAck {
		err = fmt.Errorf("Invalid ACK num. Sending ACK and dropping the packet...")
		conn.expectedSeqNum.Store(segment.TcpHeader.SeqNum + 1)
		packet := proto.NewTCPacket(conn.LocalPort, conn.RemotePort, conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck, make([]byte, 0), uint16(conn.windowSize.Load()))
		err := send(conn.t, packet, conn.LocalAddr, conn.RemoteAddr)
		if err != nil {
			return err
		}
		return nil
	}
	//TODO : check rfc for more conditions. 3.7.10.4 Handle Ack in ESTABLISHED
	if segAck > conn.sndUna.Load() && conn.sndNxt.Load() >= segAck {
		conn.sndUna.Store(segAck)
		conn.ack(segAck)
		conn.sndWnd.Store(int32(segment.TcpHeader.WindowSize))

		// TODO : can be ignored. What does it mean? do we still need to check dup ack?
		// } else if conn.sndUna.Load() >= segAck {
		// 	err = fmt.Errorf("Dup Acks. Ignoring the packet... ")
	} else if segAck > conn.sndNxt.Load() {
		conn.expectedSeqNum.Store(segment.TcpHeader.SeqNum + 1)
		packet := proto.NewTCPacket(conn.LocalPort, conn.RemotePort, conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck, make([]byte, 0), uint16(conn.windowSize.Load()))
		err := send(conn.t, packet, conn.LocalAddr, conn.RemoteAddr)
		if err != nil {
			return err
		}
		err = fmt.Errorf("Acks something not yet sent. Dropping the packet...")
	}
	return err
}

// Advance recvBuf.NXT over FIN, and sends back ACK packet
// This is not called in CLOSED, LISTEN, SYN-SENT
func handleFin(segment *proto.TCPPacket, conn *VTCPConn) error {
	conn.recvBuf.Fin(segment.TcpHeader.SeqNum)
	conn.expectedSeqNum.Add(1)
	finPacket := proto.NewTCPacket(conn.LocalPort, conn.RemotePort, conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck, make([]byte, 0), uint16(conn.windowSize.Load()))
	err := send(conn.t, finPacket, conn.LocalAddr, conn.RemoteAddr)
	if err != nil {
		return err
	}
	return nil
}

// func handleSeqNum(segment *proto.TCPPacket, conn *VTCPConn) {
// 	// 1. recv buf is full
// 	if conn.recvBuf.IsFull() {
// 		logger.Printf("receiver avail window size is 0. The segment is not acceptable.")
// 		// send ack <SEQ=SND.NXT><ACK=RCV.NXT><CTL=ACK>
// 		packet := proto.NewTCPacket(conn.LocalPort, conn.RemotePort, conn.sndNxt.Load(), conn.recvBuf.NextExpectedByte(), header.TCPFlagAck, make([]byte, 0), uint16(conn.windowSize.Load()))
// 		err := send(conn.t, packet, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
// 		if err != nil {
// 			logger.Println(err)
// 			return
// 		}
// 		logger.Printf("ack is sent. Dropping the packet...")
// 		return
// 	}

// 	// 2. TODO : Out-of-order: queue as early arrival
// 	if segment.TcpHeader.SeqNum > conn.expectedSeqNum.Load() {
// 		return
// 	}

// }

// func handleSyn(segment *proto.TCPPacket, conn *VTCPConn) {
// 	// if packet.isSyn(){

// 	// }
// }

// func handleSegText(segment *proto.TCPPacket, conn *VTCPConn) {
// 	// 1. handle normally
// 	n, err := conn.recvBuf.Write(segment.Payload) // this will write as much as possible. Trim off any additional data
// 	if err != nil {
// 		logger.Printf("failed to write to the read buffer error: %v. dropping the packet...", err)
// 		//should we drop the packet...?
// 		return
// 	}
// 	logger.Printf("received %d bytes\n", n)

// 	// 2. TODO : Out-of-order: fetch the eligible packets
// 	// 3. TODO : Send an acknowledgment of the form: <SEQ=SND.NXT><ACK=RCV.NXT><CTL=ACK>
// }
