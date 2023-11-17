package tcpstack

import (
	"fmt"
	"iptcp-nora-yu/pkg/proto"
	"time"

	"github.com/google/netstack/tcpip/header"
)

/************************************ Handle different bits of the Segment ***********************************/

// 3.10.7.4 Other states when segment arrives. It returns aggregated data to write, and aggregated seg length
func handleSeqNum(segment *proto.TCPPacket, conn *VTCPConn) ([]byte, int, error) {

	if !isValidSeg(segment, conn) {
		// send ack <SEQ=SND.NXT><ACK=RCV.NXT><CTL=ACK>
		err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
		if err != nil {
			return make([]byte, 0), 0, err
		}
		err = fmt.Errorf("Recived an unacceptable packet. Sent ACK and dropped the packet.")
		return make([]byte, 0), 0, err
	}

	segSeq := segment.TcpHeader.SeqNum
	rcvNxt := conn.expectedSeqNum.Load()

	// TODO : Queue in Early Arrivals
	if segSeq > rcvNxt {
		logger.Printf("recieved early arrival packets. Queueing...")
		conn.earlyArrivalQ.Push(&Item{
			value:    segment,
			priority: segSeq,
		})
	}

	aggSeqLen := len(segment.Payload)
	aggData := segment.Payload
	// Trimming is done in Recv Buff

	// TODO : Aggregate Early Arrivals

	return aggData, aggSeqLen, nil
}

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

// Process segment text
func handleSegText(aggData []byte, aggSegLen int, conn *VTCPConn) error {
	n, err := conn.recvBuf.Write(aggData) // this will write as much as possible. Trim off any additional data
	if err != nil {
		return err
	}
	logger.Printf("wrote %d bytes\n", n)
	conn.expectedSeqNum.Add(uint32(n))
	newWindowSize := conn.recvBuf.FreeSpace()
	conn.windowSize.Store(int32(min(BUFFER_CAPACITY, newWindowSize)))

	// sends largest contiguous ack and left-over window size back
	newTcpPacket := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
		conn.sndNxt.Load(), conn.expectedSeqNum.Load(),
		header.TCPFlagAck, make([]byte, 0), uint16(newWindowSize))
	err = send(conn.t, newTcpPacket, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
	if err != nil {
		return err
	}
	return nil
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

// func handleSyn(segment *proto.TCPPacket, conn *VTCPConn) {
// 	// if packet.isSyn(){

// 	// }
// }

/************************************ Helper Funcs ***********************************/

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

// Check if a segment is valid based on the four cases
func isValidSeg(segment *proto.TCPPacket, conn *VTCPConn) bool {
	segLen := len(segment.Payload)
	segSeq := segment.TcpHeader.SeqNum
	rcvWnd := conn.windowSize.Load()
	rcvNxt := conn.expectedSeqNum.Load()

	if segLen == 0 && rcvWnd > 0 {
		return segSeq == rcvNxt
	}

	if segLen == 0 && rcvWnd > 0 {
		return segSeq >= rcvNxt && segSeq < rcvNxt+uint32(rcvWnd)
	}

	if segLen > 0 && rcvWnd == 0 {
		return false
	}

	cond1 := segSeq >= rcvNxt && segSeq < rcvNxt+uint32(rcvWnd)
	cond2 := segSeq+uint32(segLen)-1 >= rcvNxt && segSeq+uint32(segLen)-1 < rcvNxt+uint32(rcvWnd)
	return cond1 || cond2

}

// TODO : try to refactor this
func (conn *VTCPConn) sendCTL(seq uint32, ack uint32, flag uint8) error {
	packet := proto.NewTCPacket(conn.LocalPort, conn.RemotePort, seq, ack, flag, make([]byte, 0), uint16(conn.windowSize.Load()))
	err := send(conn.t, packet, conn.LocalAddr, conn.RemoteAddr)
	if err != nil {
		return err
	}
	return nil
}
