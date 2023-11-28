package tcpstack

import (
	"container/heap"
	"iptcp-nora-yu/pkg/proto"
	"net/netip"

	"github.com/google/netstack/tcpip/header"
)

func (t *TCPGlobalInfo) isPortInUse(port uint16) bool {
	t.tableMu.RLock()
	defer t.tableMu.RUnlock()
	if _, inuse := t.listenerTable[port]; inuse {
		return true
	}
	for endpoint := range t.connTable {
		if endpoint.LocalPort == port {
			return true
		}
	}
	return false
}

func (t *TCPGlobalInfo) socketExists(endpoint TCPEndpointID) bool {
	t.tableMu.RLock()
	defer t.tableMu.RUnlock()
	_, exists := t.connTable[endpoint]
	return exists
}

func (t *TCPGlobalInfo) bindListener(port uint16, l *VTCPListener) {
	t.tableMu.Lock()
	defer t.tableMu.Unlock()
	t.listenerTable[port] = l
}

func (t *TCPGlobalInfo) bindSocket(endpoint TCPEndpointID, s *VTCPConn) {
	t.tableMu.Lock()
	defer t.tableMu.Unlock()
	t.connTable[endpoint] = s
}

func (t *TCPGlobalInfo) deleteSocket(endpoint TCPEndpointID) {
	t.tableMu.Lock()
	defer t.tableMu.Unlock()
	t.connTable[endpoint].state = CLOSED
	delete(t.connTable, endpoint)
}

func (t *TCPGlobalInfo) deleteListener(port uint16) {
	t.tableMu.Lock()
	defer t.tableMu.Unlock()
	delete(t.listenerTable, port)
}

func (t *TCPGlobalInfo) findListenerSocket(id int32) *VTCPListener {
	t.tableMu.RLock()
	defer t.tableMu.RUnlock()
	for _, l := range t.listenerTable {
		if l.socketId == id {
			return l
		}
	}
	return nil
}

func (t *TCPGlobalInfo) findNormalSocket(id int32) *VTCPConn {
	t.tableMu.RLock()
	defer t.tableMu.RUnlock()
	for _, conn := range t.connTable {
		if conn.socketId == id {
			return conn
		}
	}
	return nil
}

// Update checksum and send tcp packet
func send(t *TCPGlobalInfo, p *proto.TCPPacket, srcIP netip.Addr, destIP netip.Addr) error {
	p.TcpHeader.Checksum = 0
	p.TcpHeader.Checksum = proto.ComputeTCPChecksum(p.TcpHeader, srcIP, destIP, p.Payload)
	tcpHeaderBytes := make(header.TCP, proto.DefaultTcpHeaderLen)
	tcpHeaderBytes.Encode(p.TcpHeader)
	return t.IP.Send(destIP, append(tcpHeaderBytes, p.Payload...), uint8(proto.ProtoNumTCP))
}

// Aggregates packets in the early arrivals queue.
// Returns aggregated data and total length
func (conn *VTCPConn) aggregateEarlyArrivals(data []byte, startSeq uint32) ([]byte, int) {
	avaiWnd := conn.windowSize.Load() - int32(len(data))
	if avaiWnd == 0 {
		return data, len(data)
	}

	for conn.earlyArrivalQ.Len() != 0 {
		seg := conn.earlyArrivalQ[0].value
		segSeq := seg.TcpHeader.SeqNum
		segEnd := segSeq + uint32(len(seg.Payload))

		if segEnd < startSeq {
			logger.Debug("discarding the previous segment...", "startSEQ", segSeq, "end", segEnd)
			heap.Pop(&conn.earlyArrivalQ)
			continue
		}

		if segSeq > startSeq {
			logger.Debug("cannot merge large seg", "SEQ", segSeq)
			return data, len(data)
		}

		// at this point, segSeq <= startSeq <= segEnd. It may partially fit
		// trim the data
		start := uint32(0)
		end := min(int32(len(seg.Payload)), avaiWnd)
		if segSeq < startSeq {
			start = startSeq - segSeq
		}

		trim := seg.Payload[start:end]
		avaiWnd -= int32(len(trim))
		startSeq += uint32(len(trim))
		data = append(data, trim...)

		// if complete merge, pop
		if segEnd <= startSeq {
			heap.Pop(&conn.earlyArrivalQ)
		}
		if avaiWnd == 0 {
			return data, len(data)
		}
	}

	return data, len(data)
}
