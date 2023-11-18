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
		t:             t,
		TCPEndpointID: endpoint,
		socketId:      atomic.AddInt32(&t.socketNum, 1),
		state:         state,
		stateMu:       sync.RWMutex{},
		mu:            sync.RWMutex{},
		// srtt:           &SRTT{}, //TODO
		irs:            remoteInitSeqNum,
		iss:            iss,
		sndNxt:         &atomic.Uint32{},
		sndUna:         &atomic.Uint32{},
		sndWnd:         &atomic.Int32{},
		expectedSeqNum: &atomic.Uint32{},
		windowSize:     &atomic.Int32{},
		sendBuf:        newSendBuf(BUFFER_CAPACITY, iss),
		recvBuf:        NewRecvBuf(BUFFER_CAPACITY, remoteInitSeqNum),
		earlyArrivalQ:  PriorityQueue{},
		inflightQ:      deque.New[*packetMetadata](),
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
		w := conn.writeToSendBuf(data[bytesWritten:])
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
func (conn *VTCPConn) sendBufferedData() {
	b := conn.sendBuf
	for {
		// wait until there's new data to send
		conn.mu.RLock()
		for conn.numBytesNotSent() == 0 {
			conn.mu.RUnlock()
			<-b.hasUnsentC
			conn.mu.RLock()
		}
		b.hasUnsentC = make(chan struct{}, b.capacity)
		conn.mu.RUnlock()

		if conn.sndWnd.Load() == 0 { // zero-window probing
			oldUna := conn.sndUna.Load() // una changes -> this zwp packet has been processed
			oldNxt := conn.sndNxt.Load()
			newNxt := oldNxt + 1

			// form the packet
			conn.mu.RLock()
			byteToSend := b.buf[b.index(oldNxt)]
			conn.mu.RUnlock()
			packet := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
				oldNxt, conn.expectedSeqNum.Load(),
				header.TCPFlagAck, []byte{byteToSend}, uint16(conn.windowSize.Load()))

			// start probing
			interval := float64(1) // TODO: should be RTO
			timeout := time.NewTimer(time.Duration(interval) * time.Second)
			<-timeout.C
			for conn.sndWnd.Load() == 0 && conn.sndUna.Load() == oldUna {
				err := send(conn.t, packet, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
				if err != nil {
					logger.Println(err)
					return
				}
				conn.sndNxt.Store(newNxt)
				logger.Println("Sent zwp packet with byte: ", string(byteToSend))

				interval *= 1.3 // TODO: determine the factor
				timeout.Reset(time.Duration(interval) * time.Second)
				<-timeout.C
			}
		} else {
			numBytes, bytesToSend := conn.bytesNotSent(proto.MSS)
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
}
