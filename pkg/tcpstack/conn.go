package tcpstack

import (
	"container/heap"
	"io"
	"iptcp-nora-yu/pkg/proto"
	"sync"
	"sync/atomic"

	"github.com/google/netstack/tcpip/header"
)

// Creates a socket
func NewSocket(t *TCPGlobalInfo, state State, endpoint TCPEndpointID, remoteInitSeqNum uint32) *VTCPConn { // TODO: get rid of state? use a default init state?
	iss := generateStartSeqNum()
	conn := &VTCPConn{
		t:                t,
		TCPEndpointID:    endpoint,
		socketId:         atomic.AddInt32(&t.socketNum, 1),
		state:            state,
		stateMu:          sync.RWMutex{},
		srtt:             &SRTT{}, //TODO
		remoteInitSeqNum: remoteInitSeqNum,
		localInitSeqNum:  iss,
		seqNum:           &atomic.Uint32{},
		expectedSeqNum:   &atomic.Uint32{},
		largestAck:       &atomic.Uint32{},
		windowSize:       &atomic.Int32{},
		sendBuf:          newSendBuf(BUFFER_CAPACITY, iss),
		recvBuf:          NewCircBuff(BUFFER_CAPACITY),
		recvChan:         make(chan *proto.TCPPacket, 1),
		closeC:           make(chan struct{}, 1),
		earlyArrivalQ:    PriorityQueue{},
	}
	conn.seqNum.Store(iss)
	conn.expectedSeqNum.Store(remoteInitSeqNum + 1)
	conn.windowSize.Store(BUFFER_CAPACITY)
	heap.Init(&conn.earlyArrivalQ)
	return conn
}

// TODO: This should follow state machine CLOSE in the RFC
func (conn *VTCPConn) VClose() error {
	conn.closeC <- struct{}{}
	return nil
}

func (conn *VTCPConn) VRead(buf []byte) (int, error) {
	conn.stateMu.RLock()
	if conn.state == CLOSE_WAIT {
		conn.stateMu.RUnlock()
		return 0, io.EOF
	}
	conn.stateMu.RUnlock()
	readBuff := conn.recvBuf
	bytesRead, err := readBuff.Read(buf) //this will block if nothing to read in the buffer
	if err != nil {
		return 0, err
	}
	conn.windowSize.Add(int32(bytesRead))
	return int(bytesRead), nil
}

func (conn *VTCPConn) VWrite(data []byte) (int, error) {
	bytesWritten := conn.sendBuf.write(data) // could block
	return bytesWritten, nil                 // TODO: in what circumstances can it err?
}

/************************************ Private funcs ***********************************/

// Send out new data in the send buffer
// TODO: should only run in established state?
func (conn *VTCPConn) send() {
	for {
		bytesToSend := conn.sendBuf.send(conn.seqNum.Load(), proto.MSS) // could block
		packet := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
			conn.seqNum.Load(), conn.expectedSeqNum.Load(), //this not correct...
			header.TCPFlagAck, bytesToSend, uint16(conn.windowSize.Load()))
		err := send(conn.t, packet, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
		if err != nil {
			logger.Println(err)
			return
		}
		// update seq number
		conn.seqNum.Add(uint32(len(bytesToSend)))
	}
}

func (conn *VTCPConn) run() {
	conn.stateMu.RLock()
	defer conn.stateMu.RUnlock()
	for stateFunc := stateFuncMap[conn.state]; stateFunc != nil; stateFunc = stateFuncMap[conn.state] {
		conn.stateMu.RUnlock()
		stateFunc(conn)
		conn.stateMu.RLock()
	}
}
