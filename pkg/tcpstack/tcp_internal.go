package tcpstack

import (
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
	delete(t.connTable, endpoint)
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

// Send tcp packet
func send(t *TCPGlobalInfo, p *proto.TCPPacket, srcIP netip.Addr, destIP netip.Addr) error {
	p.TcpHeader.Checksum = 0
	p.TcpHeader.Checksum = proto.ComputeTCPChecksum(p.TcpHeader, srcIP, destIP, p.Payload)
	tcpHeaderBytes := make(header.TCP, proto.DefaultTcpHeaderLen)
	tcpHeaderBytes.Encode(p.TcpHeader)
	return t.IP.Send(destIP, append(tcpHeaderBytes, p.Payload...), uint8(proto.ProtoNumTCP))
}
