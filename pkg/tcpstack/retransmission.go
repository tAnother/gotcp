package tcpstack

import (
	"errors"
	"iptcp-nora-yu/pkg/proto"
	"time"
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

// Should be called when the retransmission timer got "switched on"
func (conn *VTCPConn) startRetransmission() {
	// TODO: lock. Also when returning from the function, should we stop the timer?
	for conn.inflightQ.Len() > 0 && !conn.RTOStatus.Load() {
		<-conn.retransTimer.C
		conn.retransmit()
		conn.RTO = min(conn.RTO*2, MAX_RTO)
		conn.retransTimer.Reset(conn.getRTODuration())
	}
}

// Resend the first packet on the queue
func (conn *VTCPConn) retransmit() error {
	inflight := conn.inflightQ.Front()
	if inflight.counter >= 3 {
		return errors.New("maximum number of retransmissions reached")
		// TODO: call active close???
	}
	err := send(conn.t, inflight.packet, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
	if err != nil {
		return err
	}
	inflight.timeSent = time.Now()
	inflight.counter++
	return nil
}
