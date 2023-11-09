package tcpstack

import (
	"io"
	"iptcp-nora-yu/pkg/proto"
	"sync"
	"sync/atomic"

	"github.com/google/netstack/tcpip/header"
)

// Creates and binds the socket
func NewSocket(t *TCPGlobalInfo, state State, endpoint TCPEndpointID, remoteInitSeqNum uint32) *VTCPConn { // TODO: get rid of state? use a default init state?
	conn := &VTCPConn{
		t:                t,
		TCPEndpointID:    endpoint,
		socketId:         atomic.AddInt32(&t.socketNum, 1),
		state:            state,
		stateMu:          sync.Mutex{},
		srtt:             &SRTT{}, //TODO
		remoteInitSeqNum: remoteInitSeqNum,
		localInitSeqNum:  generateStartSeqNum(),
		seqNum:           &atomic.Uint32{},
		expectedSeqNum:   &atomic.Uint32{},
		largestAck:       &atomic.Uint32{},
		windowSize:       &atomic.Uint32{},
		sendBuf:          newSendBuf(BUFFER_CAPACITY),
		recvBuf:          NewCircBuff(BUFFER_CAPACITY),
		recvChan:         make(chan *proto.TCPPacket, 1),
		readWait:         make(chan struct{}, 1),
	}
	conn.seqNum.Store(conn.localInitSeqNum)
	conn.windowSize.Store(BUFFER_CAPACITY)
	return conn
}

// TODO: This should follow state machine CLOSE in the RFC
func (*VTCPConn) VClose() error {
	return nil
}

func (conn *VTCPConn) VRead(buf []byte) (int, error) {
	conn.stateMu.Lock()
	if conn.state == CLOSE_WAIT {
		conn.stateMu.Unlock()
		return 0, io.EOF
	}
	conn.stateMu.Unlock()

	if !conn.recvBuf.IsEmpty() {
		bytesRead, err := conn.recvBuf.Read(buf)
		if err != nil {
			return 0, err
		}
		conn.windowSize.Add(uint32(bytesRead))
		return int(bytesRead), nil
	}

	<-conn.readWait //block if there's no available data to read

	readBuff := conn.recvBuf
	bytesRead, err := readBuff.Read(buf)
	if err != nil {
		conn.readWait = make(chan struct{}, 1) //should have a better way...
		return 0, err
	}
	conn.readWait = make(chan struct{}, 1) //should have a better way...
	conn.windowSize.Add(uint32(bytesRead))
	return int(bytesRead), nil
}

func (conn *VTCPConn) VWrite(data []byte) (int, error) {
	bytesWritten := conn.sendBuf.write(data) // could block
	return bytesWritten, nil                 // TODO: in what circumstances can it err?
}

/************************************ Private funcs ***********************************/

// Send out new data in the send buffer
func (conn *VTCPConn) send() {
	for {
		if conn.state == FIN_WAIT_1 || conn.state == LAST_ACK {
			return
		}
		bytesToSend := conn.sendBuf.send(conn.seqNum.Add(1), proto.MSS) // could block
		packet := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
			conn.seqNum.Load(), conn.expectedSeqNum.Load(),
			header.TCPFlagAck, bytesToSend, uint16(conn.windowSize.Load()))

		err := send(conn.t, packet, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
		if err != nil {
			logger.Println(err)
			return
		}
	}
}
