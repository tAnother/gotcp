package tcpstack

import (
	"fmt"
	"iptcp-nora-yu/pkg/proto"
	"time"

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

var stateFuncMap = []func(*VTCPConn, *proto.TCPPacket){
	nil,
	nil,
	stateFuncSynRcvd,
	stateFuncSynSent,
	stateFuncEstablished,
	stateFuncFinWait1,
	stateFuncFinWait2,
	stateFuncClosing,
	stateFuncTimeWait,
	stateFuncCloseWait,
	stateFuncLastAck,
}

func (conn *VTCPConn) stateMachine(segment *proto.TCPPacket) {
	conn.stateMu.RLock()
	stateFunc := stateFuncMap[conn.state]
	conn.stateMu.RUnlock()
	if stateFunc != nil {
		stateFunc(conn, segment)
	}
}

func stateFuncSynRcvd(conn *VTCPConn, segment *proto.TCPPacket) {
	_, _, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Println(err)
		return
	}

	if !segment.IsAck() {
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Printf("ACK bit is not set in SYN-RECEIVED. Dropping...")
		return
	}

	sndUna := conn.sndUna.Load()
	segAck := segment.TcpHeader.AckNum
	sndNxt := conn.sndNxt.Load()

	if sndUna < segAck && segAck <= sndNxt {
		conn.stateMu.Lock()
		conn.state = ESTABLISHED
		conn.stateMu.Unlock()
		conn.sndWnd.Store(int32(segment.TcpHeader.WindowSize))
		conn.sndUna.Store(segAck)
		go conn.sendBufferedData()
	} else {
		logger.Printf("ACK is not acceptable.")
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
	}
}

// See 3.10.7.3 SYN-SENT STATE
func stateFuncSynSent(conn *VTCPConn, packet *proto.TCPPacket) {
	// We only care SYN + ACK for simplification
	if !packet.IsAck() && !packet.IsSyn() {
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Printf("Unacceptable packet. Dropping...")
		return
	}

	segSeq := packet.TcpHeader.SeqNum
	sndNxt := conn.sndNxt.Load()
	sndUna := conn.sndUna.Load()
	segAck := packet.TcpHeader.AckNum

	//1. Check Ack bit is in the range
	if sndUna >= segAck && segAck > sndNxt {
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Printf("Unacceptable packet. Dropping...")
		return
	}

	conn.expectedSeqNum.Store(packet.TcpHeader.SeqNum + 1)
	conn.recvBuf.AdvanceNxt(segSeq, false)
	conn.recvBuf.SetLBR(segSeq)
	conn.irs = segSeq
	conn.sndUna.Store(segAck)
	sndUna = conn.sndUna.Load()

	// 3.10.7.3 fourth SYN bit
	// conn.ackInflight(segAck)

	if sndUna > conn.iss {
		conn.stateMu.Lock()
		conn.state = ESTABLISHED
		conn.stateMu.Unlock()
		conn.sndWnd.Store(int32(packet.TcpHeader.WindowSize))

		_, err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
		if err != nil {
			conn.t.deleteSocket(conn.TCPEndpointID)
			return
		}
		go conn.sendBufferedData()
	} else {
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Printf("Unacceptable packet. Dropping...")
	}
}

func stateFuncEstablished(conn *VTCPConn, segment *proto.TCPPacket) {
	aggData, aggSegLen, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Println(err)
		return
	}

	if !segment.IsAck() {
		logger.Printf("ACK bit is off. Dropping the packet...")
		return
	} else {
		err := handleAck(segment, conn)
		if err != nil {
			logger.Println(err)
			return
		}
	}

	if aggSegLen > 0 {
		err = handleSegText(aggData, aggSegLen, conn)
		if err != nil {
			logger.Println(err)
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

func stateFuncFinWait1(conn *VTCPConn, segment *proto.TCPPacket) {
	conn.stateMu.Lock()
	defer conn.stateMu.Unlock()

	aggData, aggSegLen, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Println(err)
		return
	}

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

	if aggSegLen > 0 {
		err = handleSegText(aggData, aggSegLen, conn)
		if err != nil {
			logger.Println(err)
			return
		}
	}

	if segment.IsFin() {
		if conn.sndNxt.Load() == segment.TcpHeader.AckNum {
			conn.state = TIME_WAIT
			go timeWaitTimer(conn)
		} else {
			conn.state = CLOSING
		}
	}
}

func stateFuncFinWait2(conn *VTCPConn, segment *proto.TCPPacket) {
	conn.stateMu.Lock()
	defer conn.stateMu.Unlock()

	aggData, aggSegLen, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Println(err)
		return
	}

	if !segment.IsAck() {
		logger.Printf("ACK bit is off. Dropping the packet...")
		return
	} else {
		err := handleAck(segment, conn)
		if err != nil {
			logger.Println(err)
			return
		}

		// ACK Close.
		// if the retransmission queue is empty, the user's CLOSE can be acknowledged ("ok") but do not delete the TCB.
		// TODO : not sure if my understanding is correct tho.
		if conn.inflightQ.Len() == 0 && segment.IsFin() {
			conn.expectedSeqNum.Store(segment.TcpHeader.SeqNum + 1)
			_, err = conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
			if err != nil {
				logger.Println(err)
				return
			}
		}
	}

	if aggSegLen > 0 {
		err = handleSegText(aggData, aggSegLen, conn)
		if err != nil {
			logger.Println(err)
			return
		}
	}

	if segment.IsFin() {
		conn.state = TIME_WAIT
		go timeWaitTimer(conn)
	}
}

func stateFuncCloseWait(conn *VTCPConn, segment *proto.TCPPacket) {
	_, _, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Println(err)
		return
	}

	if !segment.IsAck() {
		logger.Printf("ACK bit is off. Dropping the packet...")
	} else {
		err := handleAck(segment, conn)
		if err != nil {
			logger.Println(err)
			return
		}
	}
}

func stateFuncClosing(conn *VTCPConn, segment *proto.TCPPacket) {
	conn.stateMu.Lock()
	defer conn.stateMu.Unlock()

	_, _, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Println(err)
		return
	}

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
}

func stateFuncLastAck(conn *VTCPConn, segment *proto.TCPPacket) {
	conn.stateMu.Lock()
	defer conn.stateMu.Unlock()

	_, _, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Println(err)
		return
	}

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
			fmt.Printf("Socket %v closed", conn.socketId)
			conn.t.deleteSocket(conn.TCPEndpointID)
		} else {
			logger.Printf("Our FIN is not ACKed. Dropping the packet...")
		}
	}
}

func stateFuncTimeWait(conn *VTCPConn, segment *proto.TCPPacket) {
	conn.stateMu.Lock()
	defer conn.stateMu.Unlock()

	_, _, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Println(err)
		return
	}

	if !segment.IsAck() {
		logger.Printf("ACK bit is off. Dropping the packet...")
	} else {
		err := handleAck(segment, conn)
		if err != nil {
			logger.Println(err)
			return
		}
		conn.expectedSeqNum.Store(segment.TcpHeader.SeqNum + 1)
		_, err = conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
		if err != nil {
			logger.Println(err)
			return
		}
		conn.timeWaitReset <- true
	}

	if segment.IsFin() {
		conn.timeWaitReset <- true
	}
}

// See RFC 9293 - 3.10.4 CLOSE Call
func (conn *VTCPConn) activeClose() (err error) {
	conn.stateMu.Lock()
	//at this point, closed state and listen state are already handled
	switch conn.state {
	case SYN_SENT:
		err = fmt.Errorf("connection closing")
		conn.stateMu.Unlock()
		conn.t.deleteSocket(conn.TCPEndpointID)
	case SYN_RECEIVED:
		conn.mu.Lock()
		size := conn.numBytesNotSent()
		conn.mu.Unlock()
		// If no SENDs have been issued and there is no pending data to send
		if conn.sndNxt.Load() != conn.iss+1 && size == 0 {
			packet, err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagFin|header.TCPFlagAck)
			if err != nil {
				return err
			}
			// TODO : no need to lock the queue?
			conn.inflightQ.PushBack(&packetMetadata{length: 0, packet: packet, timeSent: time.Now()})
			// TODO : should I start the RTO?
			conn.startOrResetRetransTimer(false)

			conn.state = FIN_WAIT_1
		}
		conn.stateMu.Unlock()
	case ESTABLISHED:
		conn.mu.Lock()
		size := conn.numBytesNotSent()
		conn.mu.Unlock()

		for size > 0 {
			conn.mu.Lock()
			size = conn.numBytesNotSent()
			conn.mu.Unlock()
		}

		packet, err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagFin|header.TCPFlagAck)
		if err != nil {
			return err
		}
		// TODO : no need to lock the queue?
		conn.inflightQ.PushBack(&packetMetadata{length: 0, packet: packet, timeSent: time.Now()})
		// TODO : should I start the RTO?
		conn.startOrResetRetransTimer(false)

		conn.sndNxt.Add(1)
		conn.state = FIN_WAIT_1
		conn.stateMu.Unlock()
	case CLOSE_WAIT:
		conn.mu.Lock()
		size := conn.numBytesNotSent()
		conn.mu.Unlock()

		for size > 0 {
			conn.mu.Lock()
			size = conn.numBytesNotSent()
			conn.mu.Unlock()
		}

		packet, err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagFin|header.TCPFlagAck)
		if err != nil {
			return err
		}
		// TODO : no need to lock the queue?
		conn.inflightQ.PushBack(&packetMetadata{length: 0, packet: packet, timeSent: time.Now()})
		// TODO : should I start the RTO?
		conn.startOrResetRetransTimer(false)

		conn.sndNxt.Add(1)
		conn.state = LAST_ACK
		conn.stateMu.Unlock()
	default:
		err = fmt.Errorf("connection closing")
		conn.stateMu.Unlock()
	}
	return err
}
