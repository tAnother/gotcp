package tcpstack

import (
	"iptcp-nora-yu/pkg/proto"
	"time"

	"github.com/google/netstack/tcpip/header"
)

type packetMetadata struct {
	timeSent time.Time
	seqNum   uint32
	length   uint32
	counter  int32 // how many time it has been retransmitted
}

func (conn *VTCPConn) ackInflight(ackNum uint32) {
	for conn.inflightQ.Len() > 0 {
		meta := conn.inflightQ.Front()
		if meta.seqNum >= ackNum {
			return
		}
		if meta.seqNum+meta.length < ackNum {
			conn.inflightQ.PopFront()
		} else {
			// can also just return. the receiver should handle trimming.
			// in that case, we can simply keep the packet inside the queue, to avoid reconstructing the packet
			end := meta.seqNum + meta.length
			meta.seqNum = ackNum
			meta.length = end - ackNum
		}
	}
}

// Resend the first packet on the queue
func (conn *VTCPConn) retransmit() {
	// TODO: reset RTO

	meta := conn.inflightQ.Front()
	conn.mu.RLock()
	bytesToSend := conn.sendBuf.getBytes(meta.seqNum, meta.length)
	conn.mu.RUnlock()

	packet := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
		meta.seqNum, conn.expectedSeqNum.Load(),
		header.TCPFlagAck, bytesToSend, uint16(conn.windowSize.Load()))

	err := send(conn.t, packet, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
	if err != nil {
		logger.Println(err)
		return
	}
}
