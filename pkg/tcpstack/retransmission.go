package tcpstack

import (
	"iptcp-nora-yu/pkg/proto"
	"time"
)

const (
	MAX_RETRANSMISSIONS = 3 // R2 as in RFC 9293 - 3.8.3. We're omitting R1
)

type packetMetadata struct {
	timeSent time.Time // TODO: according to the gearup we need this, but i'm not sure why?
	// seqNum   uint32
	// length   uint32
	counter int32 // number of retransmissions sent
	packet  *proto.TCPPacket
}

func (conn *VTCPConn) ackInflight(ackNum uint32) {
	// TODO: lock!!
	for conn.inflightQ.Len() > 0 {
		meta := conn.inflightQ.Front()
		if meta.packet.TcpHeader.SeqNum >= ackNum {
			return
		}
		conn.inflightQ.PopFront()
	}
}

// Should be called only when the retransmission timer got "switched on"
func (conn *VTCPConn) startRetransmission() {
	// TODO: lock!!
	defer func() {
		conn.rtoIsRunning.Store(false)
		if !conn.retransTimer.Stop() {
			<-conn.retransTimer.C
		}
	}()

	for conn.inflightQ.Len() > 0 && conn.rtoIsRunning.Load() {
		<-conn.retransTimer.C

		// get the first packet on the queue
		inflight := conn.inflightQ.Front()
		if inflight.counter >= MAX_RETRANSMISSIONS {
			conn.inflightQ.Clear()
			go conn.activeClose() // should i ???
			return
		}

		// update rto and restart the timer
		conn.rto = min(conn.rto*2, MAX_RTO)
		conn.retransTimer.Reset(conn.getRTODuration())

		err := conn.send(inflight.packet)
		if err != nil {
			logger.Println(err)
			return
		}
		inflight.timeSent = time.Now()
		inflight.counter++
		logger.Printf("Retransmitting packet SEQ = %d (%d)...\n", inflight.packet.TcpHeader.SeqNum, inflight.counter)
	}
}
