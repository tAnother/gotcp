package tcpstack

import (
	"iptcp-nora-yu/pkg/proto"
	"time"
)

// Notes:
// The function ackInflight() implies an ordering of acquiring mutexes.
// inflightMu should never be acquired while holding rtoMu.

const (
	MAX_RETRANSMISSIONS = 3 // R2 as in RFC 9293 - 3.8.3. We're omitting R1
	MSL                 = 3 * time.Second
	MIN_RTO             = 100  // 100 ms
	MAX_RTO             = 5000 // 5 s
	ALPHA               = 0.8
	BETA                = 1.6
)

type packetMetadata struct {
	timeSent time.Time
	length   uint32
	counter  int // number of retransmission attempts
	packet   *proto.TCPPacket
}

func (conn *VTCPConn) ackInflight(ackNum uint32) {
	conn.inflightMu.Lock()
	defer conn.inflightMu.Unlock()

	acktime := time.Now()
	ackRetransmit := false

	for conn.inflightQ.Len() > 0 {
		meta := conn.inflightQ.Front()
		if meta.packet.TcpHeader.SeqNum+meta.length > ackNum {
			break
		}
		popped := conn.inflightQ.PopFront()
		logger.Debug("Acked and popped off", "SEQ", popped.packet.TcpHeader.SeqNum)

		// update SRTT only when this ACK acknowledges new data AND
		// does not acknowledge retransmitted packet
		if !ackRetransmit {
			if popped.counter != 0 {
				ackRetransmit = true
			} else {
				conn.computeRTT(float64(acktime.Sub(popped.timeSent).Milliseconds()))
			}
		}
	}

	if conn.inflightQ.Len() == 0 {
		conn.stopRetransTimer()
		logger.Debug("Empty queue! Retrans timer stopped.")
	}
}

// Handles all retransmission timeout
func (conn *VTCPConn) handleRTO() {
	for range conn.retransTimer.C {
		conn.inflightMu.Lock()
		if conn.inflightQ.Len() == 0 {
			// technically this should seldom happen. The only cases I can think of are:
			// - due to the "dummy timer start" during connection setup
			// - when timer expiration and an ACK clearing up the queue happen at the same time
			conn.inflightMu.Unlock()
			conn.rtoMu.Lock()
			conn.rtoIsRunning = false
			conn.rtoMu.Unlock()
			continue
		}

		// get the first packet on the queue
		inflight := conn.inflightQ.Front()
		if inflight.counter >= MAX_RETRANSMISSIONS {
			conn.inflightQ.Clear()
			conn.inflightMu.Unlock()
			// clear send buffer
			conn.mu.Lock()
			conn.sendBuf.lbw = conn.sndNxt.Load()
			conn.sndUna.Store(conn.sndNxt.Load())
			conn.mu.Unlock()
			conn.t.deleteSocket(conn.TCPEndpointID)
			logger.Info("Max retransmission attempts reached. Connection teared down.")
			return
		}
		conn.inflightMu.Unlock()

		logger.Debug("Retransmitting packet...", "SEQ", inflight.packet.TcpHeader.SeqNum, "attempt", inflight.counter)

		// update rto and restart the timer
		conn.rtoMu.Lock()
		conn.rto = min(conn.rto*2, MAX_RTO)
		conn.retransTimer.Reset(conn.getRTODuration())
		conn.rtoMu.Unlock()

		// update packet info and retransmit
		inflight.packet.TcpHeader.AckNum = conn.expectedSeqNum.Load()
		inflight.packet.TcpHeader.WindowSize = uint16(conn.windowSize.Load())
		if err := conn.send(inflight.packet); err != nil {
			logger.Error(err.Error())
			return
		}
		inflight.timeSent = time.Now()
		inflight.counter++
	}
}

// *************************************** Helper funcs ******************************************

// Set retrans timer to expire after RTO.
// forced: whether to reset when the timer is already running
func (conn *VTCPConn) resetRetransTimer(forced bool) {
	conn.rtoMu.Lock()
	defer conn.rtoMu.Unlock()
	if !forced && conn.rtoIsRunning {
		return
	}

	if conn.rtoIsRunning && !conn.retransTimer.Stop() {
		<-conn.retransTimer.C
	}
	conn.retransTimer.Reset(conn.getRTODuration())
	logger.Debug("timer reset!")
}

func (conn *VTCPConn) stopRetransTimer() {
	conn.rtoMu.Lock()
	defer conn.rtoMu.Unlock()

	if conn.rtoIsRunning && !conn.retransTimer.Stop() {
		<-conn.retransTimer.C
	}
	conn.rtoIsRunning = false
}

// RFC 793
func (conn *VTCPConn) computeRTT(r float64) {
	conn.rtoMu.Lock()
	defer conn.rtoMu.Unlock()

	if conn.firstRTT {
		conn.sRTT = r
		conn.firstRTT = false
	} else {
		conn.sRTT = (ALPHA * conn.sRTT) + (1-ALPHA)*r
	}
	conn.rto = max(MIN_RTO, min(BETA*conn.sRTT, MAX_RTO))
}

// Set the timer status to be running and computes RTO duration for the timer.
// This should be called whenever starting or reseting the retrans timer.
// conn.rtoMu should be locked before entry.
func (conn *VTCPConn) getRTODuration() time.Duration {
	conn.rtoIsRunning = true
	return time.Duration(conn.rto * float64(time.Millisecond))
}
