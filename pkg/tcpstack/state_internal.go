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
	logger.Debug("Received TCP packet", "state", stateString[conn.state],
		"SEQ", segment.TcpHeader.SeqNum, "ACK", segment.TcpHeader.AckNum, "WIN", segment.TcpHeader.WindowSize,
		"Flags", proto.TCPFlagsAsString(segment.TcpHeader.Flags), "PayloadLen", len(segment.Payload), "Payload", string(segment.Payload))

	if !isValidSeg(segment, conn) {
		err := fmt.Errorf("received an unacceptable packet. Send ACK and dropped the packet")
		_, e := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
		if e != nil {
			logger.Error(e.Error())
		}
		return make([]byte, 0), 0, err
	}

	segSeq := segment.TcpHeader.SeqNum
	rcvNxt := conn.expectedSeqNum.Load()

	if segSeq > rcvNxt {
		logger.Debug("received early arrival packets. Queueing...")
		conn.earlyArrivalQ.Push(&Item{
			value:    segment,
			priority: segSeq,
		})
		_, e := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
		if e != nil {
			logger.Error(e.Error())
		}
		return make([]byte, 0), 0, nil
	}

	aggSeqLen := len(segment.Payload)
	rcvWnd := conn.windowSize.Load()
	aggData := segment.Payload

	// Trim off any data of this segment that lies outside the window (before and after)
	start := uint32(0)
	end := min(int32(aggSeqLen), rcvWnd)
	if segSeq < rcvNxt {
		start = rcvNxt - segSeq
	}

	aggData = aggData[start:end]

	//Aggregate Early Arrivals. This returns fittable aggregated data.
	aggData, aggSeqLen = conn.aggregateEarlyArrivals(aggData, segSeq+uint32(len(aggData)))

	return aggData, aggSeqLen, nil
}

// Preprocess ACK bit
func handleAck(segment *proto.TCPPacket, conn *VTCPConn) (err error) {
	segAck := segment.TcpHeader.AckNum
	if conn.sndUna.Load()-uint32(BUFFER_CAPACITY) > segAck || // BUFFER_CAPACITY being the hardcoded MAX.SND.WND
		conn.sndNxt.Load() < segAck {
		_, err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
		if err != nil {
			logger.Error(err.Error())
		}
		return fmt.Errorf("(sndnxt=%d, snduna=%d, ack=%d) Invalid ACK num. Packet dropped", conn.sndNxt.Load(), conn.sndUna.Load(), segAck)
	}
	logger.Debug("Valid ack num", "ACK", segAck, "SND.NXT", conn.sndNxt.Load(), "SND.UNA", conn.sndUna.Load())

	if conn.sndUna.Load() < segAck && segAck <= conn.sndNxt.Load() {
		// seems like we need to ensure SND.UNA & SND.WND are updated atomically. Not sure why
		conn.mu.Lock()
		conn.sndUna.Store(segAck)
		conn.sndWnd.Store(int32(segment.TcpHeader.WindowSize))
		conn.sendBuf.freespaceC <- struct{}{}
		conn.mu.Unlock()
		// calling reset before ackInflight prevents the timer
		// that is stopped in ackInflight to be switched on again
		// (at the cost of not using the newest SRTT for timeout)
		conn.resetRetransTimer(true)
		conn.ackInflight(segAck)

	} else if conn.sndUna.Load() == segAck {
		conn.sndWnd.Store(int32(segment.TcpHeader.WindowSize))
		// RFC 6298 suggests timer to reset only when new data is acked.
		// We reset it here as well, as we don't want any segment to
		// reach max retransmissions during zero-window probing
		conn.resetRetransTimer(true)
	} else if segAck > conn.sndNxt.Load() {
		err = fmt.Errorf("acking something not yet sent. Packet dropped")
		_, e := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
		if e != nil {
			logger.Error(e.Error())
		}
	}
	return err
}

// Process segment text
func handleSegText(aggData []byte, aggSegLen int, conn *VTCPConn) error {
	n, err := conn.recvBuf.Write(aggData) // this will write as much as possible. Trim off any additional data
	if err != nil {
		return err
	}
	conn.expectedSeqNum.Add(uint32(n))
	newWindowSize := conn.recvBuf.FreeSpace()
	conn.windowSize.Store(int32(min(BUFFER_CAPACITY, newWindowSize)))

	// sends largest contiguous ack and left-over window size back
	if _, err = conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck); err != nil {
		return err
	}
	return nil
}

// Advance recvBuf.NXT over FIN, and sends back ACK packet
func handleFin(segment *proto.TCPPacket, conn *VTCPConn) error {
	conn.recvBuf.AdvanceNxt(segment.TcpHeader.SeqNum, true)
	conn.expectedSeqNum.Add(1)
	_, err := conn.sendCTL(conn.sndNxt.Load(), conn.expectedSeqNum.Load(), header.TCPFlagAck)
	if err != nil {
		return err
	}
	return nil
}

/************************************ Helper Funcs ***********************************/

// Timer for TIME-WAIT
func timeWaitTimer(conn *VTCPConn) {
	timer := time.NewTimer(2 * MSL)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			// expires
			conn.setState(CLOSED)
			fmt.Printf("Socket %v closed", conn.socketId)
			conn.t.deleteSocket(TCPEndpointID{LocalAddr: conn.LocalAddr, RemoteAddr: conn.RemoteAddr, LocalPort: conn.LocalPort, RemotePort: conn.RemotePort})
			return
		case <-conn.timeWaitReset:
			if !timer.Stop() {
				<-timer.C // Drain the channel if the timer already expired
			}
			timer.Reset(2 * MSL)
		}
	}
}

// Check if a segment is valid based on the four cases
func isValidSeg(segment *proto.TCPPacket, conn *VTCPConn) bool {
	segLen := len(segment.Payload)
	segSeq := segment.TcpHeader.SeqNum
	rcvWnd := conn.windowSize.Load()
	rcvNxt := conn.expectedSeqNum.Load()

	if segLen == 0 && rcvWnd == 0 {
		return segSeq == rcvNxt
	}

	if segLen == 0 && rcvWnd > 0 {
		return segSeq >= rcvNxt && segSeq < rcvNxt+uint32(rcvWnd)
	}

	if segLen > 0 && rcvWnd == 0 {
		// treat as zero-window probing
		return false
	}

	// check if seg seq is partially inside our recv window
	cond1 := segSeq >= rcvNxt && segSeq < rcvNxt+uint32(rcvWnd)
	cond2 := segSeq+uint32(segLen)-1 >= rcvNxt && segSeq+uint32(segLen)-1 < rcvNxt+uint32(rcvWnd)
	return cond1 || cond2
}
