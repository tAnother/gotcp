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
	stateFunc := stateFuncMap[conn.getState()]
	if stateFunc != nil {
		stateFunc(conn, segment)
	}
}

func stateFuncSynRcvd(conn *VTCPConn, segment *proto.TCPPacket) {
	aggData, aggSegLen, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Debug(err.Error())
		return
	}

	if !segment.IsAck() {
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Debug("ACK bit is not set in SYN-RECEIVED. Dropping...")
		return
	}

	sndUna := conn.sndUna.Load()
	segAck := segment.TcpHeader.AckNum
	sndNxt := conn.sndNxt.Load()

	if sndUna < segAck && segAck <= sndNxt {
		conn.setState(ESTABLISHED)
		conn.sndWnd.Store(int32(segment.TcpHeader.WindowSize))
		// below is to simulate "continue processing in ESTABLISHED state" (RFC 9293 - 3.10.7.4)
		conn.sndUna.Store(segAck)
		if aggSegLen > 0 {
			err := handleSegText(aggData, aggSegLen, conn)
			if err != nil {
				logger.Debug(err.Error())
				return
			}
		}
		go conn.sendBufferedData()
	} else {
		logger.Debug("ACK is not acceptable.")
	}

	if segment.IsFin() {
		err := handleFin(segment, conn)
		if err != nil {
			logger.Debug("error handling fin packet")
			return
		}
		conn.setState(CLOSE_WAIT)
	}
}

// See 3.10.7.3 SYN-SENT STATE
func stateFuncSynSent(conn *VTCPConn, packet *proto.TCPPacket) {
	// We do not handle simultaneous openining, so we only care SYN + ACK for simplification
	if !packet.IsAck() && !packet.IsSyn() {
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Debug("Unacceptable packet. Dropping...")
		return
	}

	segSeq := packet.TcpHeader.SeqNum
	sndNxt := conn.sndNxt.Load()
	sndUna := conn.sndUna.Load()
	segAck := packet.TcpHeader.AckNum

	// check Ack bit is in the range
	if sndUna >= segAck && segAck > sndNxt {
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Debug("Unacceptable packet. Dropping...")
		return
	}

	conn.expectedSeqNum.Store(packet.TcpHeader.SeqNum + 1)
	conn.recvBuf.AdvanceNxt(segSeq, false)
	conn.recvBuf.SetLBR(segSeq)
	conn.irs = segSeq
	conn.sndUna.Store(segAck)
	sndUna = conn.sndUna.Load()

	if sndUna > conn.iss {
		conn.setState(ESTABLISHED)
		conn.sndWnd.Store(int32(packet.TcpHeader.WindowSize))

		_, err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
		if err != nil {
			conn.t.deleteSocket(conn.TCPEndpointID)
			return
		}
		go conn.sendBufferedData()
	} else {
		conn.t.deleteSocket(conn.TCPEndpointID)
		logger.Debug("Unacceptable packet. Dropping...")
	}
}

func stateFuncEstablished(conn *VTCPConn, segment *proto.TCPPacket) {
	aggData, aggSegLen, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Debug(err.Error())
		return
	}

	if !segment.IsAck() {
		logger.Debug("ACK bit is off. Dropping the packet...")
		return
	}

	if err := handleAck(segment, conn); err != nil {
		logger.Debug(err.Error())
		return
	}

	if aggSegLen > 0 {
		err = handleSegText(aggData, aggSegLen, conn)
		if err != nil {
			logger.Debug(err.Error())
			return
		}
	}

	if segment.IsFin() {
		err := handleFin(segment, conn)
		if err != nil {
			logger.Debug("error handling fin packet")
			return
		}
		conn.setState(CLOSE_WAIT)
		return
	}
}

func stateFuncFinWait1(conn *VTCPConn, segment *proto.TCPPacket) {
	aggData, aggSegLen, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Debug(err.Error())
		return
	}

	if !segment.IsAck() {
		logger.Debug("ACK bit is off. Dropping the packet...")
		return
	}

	if err := handleAck(segment, conn); err != nil {
		logger.Debug(err.Error())
		return
	}

	// check if fin is acked
	if segment.TcpHeader.AckNum == conn.sndNxt.Load() {
		if segment.IsFin() {
			conn.setState(TIME_WAIT)
			go timeWaitTimer(conn)
			return
		}
		conn.setState(FIN_WAIT_2)
	}
	if segment.IsFin() {
		conn.setState(CLOSING)
	}

	if aggSegLen > 0 {
		err = handleSegText(aggData, aggSegLen, conn)
		if err != nil {
			logger.Debug(err.Error())
			return
		}
	}
}

func stateFuncFinWait2(conn *VTCPConn, segment *proto.TCPPacket) {
	aggData, aggSegLen, err := handleSeqNum(segment, conn)
	if err != nil {
		logger.Debug(err.Error())
		return
	}

	if !segment.IsAck() {
		logger.Debug("ACK bit is off. Dropping the packet...")
		return
	}

	if err := handleAck(segment, conn); err != nil {
		logger.Debug(err.Error())
		return
	}

	// TODO : not sure if my understanding is correct tho.
	// seems like the only events FIN-WAIT-2 must handle are:
	// 1. acking whatever is sent from the other side
	// 2. is isFin(), transit to TIME-WAIT
	// so maybe it is safe to not look at the inflightQ at all
	if conn.inflightQ.Len() == 0 && segment.IsFin() {
		conn.expectedSeqNum.Store(segment.TcpHeader.SeqNum + 1)
		_, err = conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
		if err != nil {
			logger.Error(err.Error())
			return
		}
	}

	if aggSegLen > 0 {
		err = handleSegText(aggData, aggSegLen, conn)
		if err != nil {
			logger.Debug(err.Error())
			return
		}
	}

	if segment.IsFin() {
		conn.setState(TIME_WAIT)
		go timeWaitTimer(conn)
	}
}

func stateFuncCloseWait(conn *VTCPConn, segment *proto.TCPPacket) {
	if _, _, err := handleSeqNum(segment, conn); err != nil {
		logger.Debug(err.Error())
		return
	}

	if !segment.IsAck() {
		logger.Debug("ACK bit is off. Dropping the packet...")
		return
	}

	if err := handleAck(segment, conn); err != nil {
		logger.Debug(err.Error())
		return
	}
}

func stateFuncClosing(conn *VTCPConn, segment *proto.TCPPacket) {
	if _, _, err := handleSeqNum(segment, conn); err != nil {
		logger.Debug(err.Error())
		return
	}

	if !segment.IsAck() {
		logger.Debug("ACK bit is off. Dropping the packet...")
		return
	}

	if err := handleAck(segment, conn); err != nil {
		logger.Debug(err.Error())
		return
	}

	if conn.sndNxt.Load() != segment.TcpHeader.AckNum {
		logger.Debug("Our FIN is not ACKed. Dropping the packet...")
		return
	}
	conn.setState(TIME_WAIT)
	go timeWaitTimer(conn)
}

func stateFuncLastAck(conn *VTCPConn, segment *proto.TCPPacket) {
	if _, _, err := handleSeqNum(segment, conn); err != nil {
		logger.Debug(err.Error())
		return
	}

	if !segment.IsAck() {
		logger.Debug("ACK bit is off. Dropping the packet...")
		return
	}

	if err := handleAck(segment, conn); err != nil {
		logger.Debug(err.Error())
		return
	}

	if conn.sndNxt.Load() != segment.TcpHeader.AckNum {
		logger.Debug("Our FIN is not ACKed. Dropping the packet...")
		return
	}
	conn.setState(CLOSED)
	fmt.Printf("Socket %v closed", conn.socketId)
	conn.t.deleteSocket(conn.TCPEndpointID)
}

func stateFuncTimeWait(conn *VTCPConn, segment *proto.TCPPacket) {
	if _, _, err := handleSeqNum(segment, conn); err != nil {
		logger.Debug(err.Error())
		return
	}

	if !segment.IsAck() {
		logger.Debug("ACK bit is off. Dropping the packet...")
		return
	}

	if err := handleAck(segment, conn); err != nil {
		logger.Debug(err.Error())
		return
	}
	conn.expectedSeqNum.Store(segment.TcpHeader.SeqNum + 1)
	_, err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
	if err != nil {
		logger.Debug(err.Error())
		return
	}
	conn.timeWaitReset <- true
}

// See RFC 9293 - 3.10.4 CLOSE Call
// TODO: see if there's way to get rid of spin wait
func (conn *VTCPConn) activeClose() (err error) {
	state := conn.getState()
	// at this point, closed state and listen state are already handled
	switch state {
	case SYN_SENT:
		err = fmt.Errorf("connection closing")
		conn.t.deleteSocket(conn.TCPEndpointID)
	case SYN_RECEIVED:
		conn.mu.RLock()
		size := conn.numBytesNotSent()
		conn.mu.RUnlock()
		// If no SENDs have been issued and there is no pending data to send
		if conn.sndNxt.Load() != conn.iss+1 && size == 0 {
			packet, err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagFin|header.TCPFlagAck)
			if err != nil {
				return err
			}
			// TODO : no need to lock the queue? need to start the RTO?
			conn.inflightQ.PushBack(&packetMetadata{length: 0, packet: packet, timeSent: time.Now()})
			conn.resetRetransTimer(false)

			conn.setState(FIN_WAIT_1)
		}
	case ESTABLISHED:
		conn.mu.RLock()
		size := conn.numBytesNotSent()
		conn.mu.RUnlock()

		for size > 0 {
			conn.mu.RLock()
			size = conn.numBytesNotSent()
			conn.mu.RUnlock()
		}

		packet, err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagFin|header.TCPFlagAck)
		if err != nil {
			return err
		}
		conn.inflightQ.PushBack(&packetMetadata{length: 0, packet: packet, timeSent: time.Now()})
		conn.resetRetransTimer(false)

		conn.sndNxt.Add(1)
		conn.setState(FIN_WAIT_1)
	case CLOSE_WAIT:
		conn.mu.RLock()
		size := conn.numBytesNotSent()
		conn.mu.RUnlock()

		for size > 0 {
			conn.mu.RLock()
			size = conn.numBytesNotSent()
			conn.mu.RUnlock()
		}

		packet, err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagFin|header.TCPFlagAck)
		if err != nil {
			return err
		}
		conn.inflightQ.PushBack(&packetMetadata{length: 0, packet: packet, timeSent: time.Now()})
		conn.resetRetransTimer(false)

		conn.sndNxt.Add(1)
		conn.setState(LAST_ACK)
	default:
		err = fmt.Errorf("connection closing")
	}
	return err
}
