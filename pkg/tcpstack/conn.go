package tcpstack

import (
	"iptcp-nora-yu/pkg/proto"
	"sync"
	"sync/atomic"
)

// Creates and binds the socket
func NewSocket(state State, endpoint TCPEndpointID, remoteInitSeqNum uint32) *VTCPConn {
	conn := &VTCPConn{
		socketId:   atomic.AddInt32(&socketId, 1),
		localAddr:  endpoint.localAddr,
		localPort:  endpoint.localPort,
		remoteAddr: endpoint.remoteAddr,
		remotePort: endpoint.remotePort,
		sendBuff:   proto.NewCircBuff(BUFFER_CAPACITY),
		recvBuff:   proto.NewCircBuff(BUFFER_CAPACITY),
		state:      state,
		stateMu:    sync.Mutex{},

		//TODO
		srtt:             &proto.SRTT{},
		remoteInitSeqNum: remoteInitSeqNum,
		localInitSeqNum:  generateStartSeqNum(),
		expectedSeqNum:   0,
		largestAckedNum:  0,
		windowSize:       0,
		recvChan:         make(chan *proto.TCPPacket),
	}
	return conn
}

// func (*VTCPConn) VRead(buf []byte) (int, error)   // not for milestone I
// func (*VTCPConn) VWrite(data []byte) (int, error) // not for milestone I
// func (*VTCPConn) VClose() error                   // not for milestone I

// TODO: Placeholder
func (*VTCPConn) handleConnection() {}

func (v *VTCPConn) setState(state State) {
	v.stateMu.Lock()
	defer v.stateMu.Unlock()
	v.state = state
}
