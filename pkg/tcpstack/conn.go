package tcpstack

import (
	"container/heap"
	"errors"
	"fmt"
	"io"
	"iptcp-nora-yu/pkg/proto"
	"sync"
	"sync/atomic"
	"time"

	deque "github.com/gammazero/deque"
	"github.com/google/netstack/tcpip/header"
)

// Creates a socket
func NewSocket(t *TCPGlobalInfo, state State, endpoint TCPEndpointID, remoteInitSeqNum uint32) *VTCPConn { // TODO: get rid of state? use a default init state?
	iss := generateStartSeqNum()
	conn := &VTCPConn{
		t:              t,
		TCPEndpointID:  endpoint,
		socketId:       atomic.AddInt32(&t.socketNum, 1),
		state:          state,
		stateMu:        sync.RWMutex{},
		srtt:           &SRTT{}, //TODO
		irs:            remoteInitSeqNum,
		iss:            iss,
		sndNxt:         &atomic.Uint32{},
		sndUna:         &atomic.Uint32{},
		sndWnd:         &atomic.Int32{},
		expectedSeqNum: &atomic.Uint32{},
		windowSize:     &atomic.Int32{},
		sendBuf:        newSendBuf(BUFFER_CAPACITY, iss),
		recvBuf:        NewCircBuff(BUFFER_CAPACITY, remoteInitSeqNum),
		earlyArrivalQ:  PriorityQueue{},
		inflightQ:      deque.New[*proto.TCPPacket](),
		recvChan:       make(chan *proto.TCPPacket, 1),
		closeC:         make(chan struct{}, 1),
		timeWaitReset:  make(chan bool),
	}
	conn.sndNxt.Store(iss)
	conn.sndUna.Store(iss)
	conn.expectedSeqNum.Store(remoteInitSeqNum + 1)
	conn.windowSize.Store(BUFFER_CAPACITY)
	heap.Init(&conn.earlyArrivalQ)
	return conn
}

func (conn *VTCPConn) VClose() error {
	return conn.activeClose()
}

func (conn *VTCPConn) VRead(buf []byte) (int, error) {
	conn.stateMu.RLock()
	if conn.state == CLOSE_WAIT {
		conn.stateMu.RUnlock()
		return 0, io.EOF
	}
	if conn.state == FIN_WAIT_2 || conn.state == FIN_WAIT_1 || conn.state == CLOSING {
		conn.stateMu.RUnlock()
		return 0, fmt.Errorf("operation not permitted")
	}
	conn.stateMu.RUnlock()
	readBuff := conn.recvBuf
	bytesRead, err := readBuff.Read(buf) // this will block if nothing to read in the buffer
	if err != nil {
		return 0, err
	}
	conn.windowSize.Add(int32(bytesRead))
	return int(bytesRead), nil
}

func (conn *VTCPConn) VWrite(data []byte) (int, error) {
	bytesWritten := 0
	for bytesWritten < len(data) {
		conn.stateMu.RLock()
		if conn.state == FIN_WAIT_1 || conn.state == FIN_WAIT_2 || conn.state == CLOSING || conn.state == TIME_WAIT {
			conn.stateMu.RUnlock()
			return bytesWritten, errors.New("trying to write to a closing connection")
		}
		conn.stateMu.RUnlock()
		w := conn.write(data[bytesWritten:])
		bytesWritten += w
	}
	return bytesWritten, nil
}

/************************************ Private funcs ***********************************/

func (conn *VTCPConn) run() {
	for {
		segment := <-conn.recvChan
		conn.stateMachine(segment)
	}
}

// Send out new data in the send buffer
// TODO: should only run in established state?
func (conn *VTCPConn) send() {
	for {
		if conn.sndWnd.Load() == 0 {
			conn.zeroWindowProbe()
		}
		numBytes, bytesToSend := conn.bytesNotSent(proto.MSS, false) // could block
		if numBytes == 0 {
			continue
		}
		packet := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
			conn.sndNxt.Load(), conn.expectedSeqNum.Load(),
			header.TCPFlagAck, bytesToSend, uint16(conn.windowSize.Load()))
		err := send(conn.t, packet, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
		if err != nil {
			logger.Println(err)
			return
		}
		logger.Println("Sent packet with bytes: ", string(bytesToSend))
		// conn.inflightQ.PushBack(packet)
		// update seq number
		conn.sndNxt.Add(uint32(numBytes))
	}
}

func (conn *VTCPConn) zeroWindowProbe() {
	_, bytesToSend := conn.bytesNotSent(1, true)
	oldSndNxt := conn.sndNxt.Load()
	newSndNxt := oldSndNxt + 1
	packet := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
		oldSndNxt, conn.expectedSeqNum.Load(),
		header.TCPFlagAck, bytesToSend, uint16(conn.windowSize.Load()))

	interval := float64(1) // TODO: should be RTO
	timeout := time.NewTimer(time.Duration(interval) * time.Second)
	for conn.sndWnd.Load() == 0 {
		<-timeout.C

		err := send(conn.t, packet, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
		if err != nil {
			logger.Println(err)
			return
		}
		conn.sndNxt.Store(newSndNxt)
		logger.Println("Sent zwp packet with byte: ", string(bytesToSend))

		interval *= 1.1 // TODO: determine the factor
		timeout.Reset(time.Duration(interval) * time.Second)
	}
}
