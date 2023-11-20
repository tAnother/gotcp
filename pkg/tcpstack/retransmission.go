package tcpstack

import (
	"iptcp-nora-yu/pkg/proto"
	"time"

	"github.com/google/netstack/tcpip/header"
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
	}
}

// Handles all retransmission timeout
func (conn *VTCPConn) handleRTO() {
	for range conn.retransTimer.C {
		conn.inflightMu.Lock()
		if conn.inflightQ.Len() == 0 { // TODO: technically this should never happen. what to do in this case then?
			conn.inflightMu.Unlock()
			return
		}

		// get the first packet on the queue
		inflight := conn.inflightQ.Front()
		if inflight.counter >= MAX_RETRANSMISSIONS {
			conn.inflightQ.Clear()
			conn.inflightMu.Unlock()
			go conn.activeClose() // should i ???
			return
		}
		conn.inflightMu.Unlock()

		// update rto and restart the timer
		conn.rtoMu.Lock()
		conn.rto = min(conn.rto*2, MAX_RTO)
		conn.retransTimer.Reset(conn.getRTODuration())
		conn.rtoMu.Unlock()

		// update packet info and retransmit
		inflight.packet.TcpHeader.AckNum = conn.expectedSeqNum.Load()
		inflight.packet.TcpHeader.WindowSize = uint16(conn.windowSize.Load())
		if err := conn.send(inflight.packet); err != nil {
			logger.Println(err)
			return
		}
		inflight.timeSent = time.Now()
		inflight.counter++
		logger.Printf("Retransmitting packet SEQ = %d (Attempt %d)...\n", inflight.packet.TcpHeader.SeqNum, inflight.counter)
	}
}

func (conn *VTCPConn) handshakeRetrans(i int, isActive bool) bool {
	logger.Printf("Handshake attempt %d\n", i)
	if isActive {
		_, err := conn.sendCTL(conn.iss, 0, header.TCPFlagSyn)
		if err != nil {
			return false
		}
	} else {
		_, err := conn.sendCTL(conn.iss, conn.expectedSeqNum.Load(), header.TCPFlagSyn|header.TCPFlagAck)
		if err != nil {
			return false
		}
	}

	timer := time.NewTimer(time.Duration((i + 1) * int(time.Second)))
	for {
		select {
		case <-timer.C:
			return false
		case segment := <-conn.recvChan:
			conn.stateMachine(segment)
			return true
		}
	}
}

// *************************************** Helper funcs ******************************************

// Set retrans timer to expire after RTO.
// forced: whether to reset when the timer is already running
func (conn *VTCPConn) startOrResetRetransTimer(forced bool) {
	conn.rtoMu.Lock()
	defer conn.rtoMu.Unlock()
	if !forced && conn.rtoIsRunning {
		return
	}

	if conn.retransTimer == nil {
		conn.retransTimer = time.NewTimer(conn.getRTODuration())
	} else {
		if !conn.retransTimer.Stop() {
			<-conn.retransTimer.C
		}
		conn.retransTimer.Reset(conn.getRTODuration())
	}
}

func (conn *VTCPConn) stopRetransTimer() {
	conn.rtoMu.Lock()
	defer conn.rtoMu.Unlock()

	conn.rtoMu.Lock()
	if !conn.retransTimer.Stop() {
		<-conn.retransTimer.C
	}
	conn.rtoIsRunning = false
	conn.rtoMu.Unlock()
}

// RFC 793
func (conn *VTCPConn) computeRTT(r float64) {
	conn.rtoMu.Lock()
	defer conn.rtoMu.Unlock()

	if conn.firstRTT.Load() {
		conn.sRTT = r
		conn.firstRTT.Store(false)
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
