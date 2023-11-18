package tcpstack

import (
	"errors"
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
	// TODO: lock. Also when returning from the function, should we stop the timer?
	defer conn.RTOStatus.Store(false)
	defer conn.retransTimer.Stop()
	for conn.inflightQ.Len() > 0 && conn.RTOStatus.Load() {
		<-conn.retransTimer.C
		err := conn.retransmit()
		if err != nil {
			go conn.activeClose() // should i ???
			return
		}
		conn.RTO = min(conn.RTO*2, MAX_RTO)
		conn.retransTimer.Reset(conn.getRTODuration())
	}
}

// Resend the first packet on the queue
func (conn *VTCPConn) retransmit() error {
	inflight := conn.inflightQ.Front()
	if inflight.counter >= MAX_RETRANSMISSIONS {
		return errors.New("maximum number of retransmissions reached")
	}
	err := conn.send(inflight.packet)
	if err != nil {
		return err
	}
	inflight.timeSent = time.Now()
	inflight.counter++
	return nil
}
