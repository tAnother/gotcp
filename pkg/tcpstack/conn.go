package tcpstack

import (
	"container/heap"
	"errors"
	"fmt"
	"io"
	"iptcp-nora-yu/pkg/proto"
	"time"

	"github.com/gammazero/deque"
	"github.com/google/netstack/tcpip/header"
)

// Creates a socket
func NewSocket(t *TCPGlobalInfo, state State, endpoint TCPEndpointID, remoteInitSeqNum uint32) *VTCPConn {
	iss := generateStartSeqNum()
	conn := &VTCPConn{
		t:             t,
		TCPEndpointID: endpoint,
		socketId:      t.socketNum.Add(1),
		state:         state,
		irs:           remoteInitSeqNum,
		iss:           iss,
		sendBuf:       newSendBuf(BUFFER_CAPACITY, iss),
		recvBuf:       NewRecvBuf(BUFFER_CAPACITY, remoteInitSeqNum),
		earlyArrivalQ: PriorityQueue{},
		recvChan:      make(chan *proto.TCPPacket, 1),
		timeWaitReset: make(chan bool),

		retransTimer: time.NewTimer(MIN_RTO),
		rto:          1000, // before a RTT is measured, set RTO to 1 second = 1000 ms
		firstRTT:     true,
		inflightQ:    deque.New[*packetMetadata](),
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
	readBuff := conn.recvBuf
	curState := conn.getState()
	if curState == CLOSE_WAIT || curState == LAST_ACK {
		if readBuff.WindowSize() == 0 {
			return 0, io.EOF
		}
		// consume the bytes sent before FIN
		bytesRead, err := readBuff.Read(buf)
		if err != nil {
			return 0, err
		}
		return int(bytesRead), io.EOF
	}
	if curState == FIN_WAIT_2 || curState == FIN_WAIT_1 || curState == CLOSING {
		return 0, fmt.Errorf("operation not permitted")
	}
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
		curState := conn.getState()
		if curState == FIN_WAIT_1 || curState == FIN_WAIT_2 || curState == CLOSING || curState == TIME_WAIT {
			return bytesWritten, errors.New("trying to write to a closing connection")
		}
		w := conn.writeToSendBuf(data[bytesWritten:])
		bytesWritten += w
	}
	return bytesWritten, nil
}

/************************************ Private funcs ***********************************/

func (conn *VTCPConn) getState() State {
	conn.stateMu.RLock()
	s := conn.state
	conn.stateMu.RUnlock()
	return s
}

func (conn *VTCPConn) setState(state State) {
	conn.stateMu.Lock()
	conn.state = state
	conn.stateMu.Unlock()
}

// To handle connection handshake & possible retransmissions
func (conn *VTCPConn) doHandshakes(attempt int, isActive bool) (succeed bool) {
	if attempt == MAX_RETRANSMISSIONS+1 {
		return false
	}
	fmt.Printf("Handshake attempt %d\n", attempt)
	if isActive {
		if _, err := conn.sendCTL(conn.iss, 0, header.TCPFlagSyn); err != nil {
			return false
		}
	} else {
		if _, err := conn.sendCTL(conn.iss, conn.expectedSeqNum.Load(), header.TCPFlagSyn|header.TCPFlagAck); err != nil {
			return false
		}
	}

	timer := time.NewTimer(time.Duration((attempt + 1) * int(time.Second)))
	for {
		select {
		case <-timer.C:
			conn.rto = 3000 // reinit RTO to 3s if handshake ever times out
			return conn.doHandshakes(attempt+1, isActive)
		case segment := <-conn.recvChan:
			if !isActive && segment.IsSyn() {
				// Deviation from RFC 9293:
				// Instead of checking SEQ num, we handle SYN first.
				// All segment that can reach here have the same addr &
				// port, thus we just reuse the conn for convenience,
				// with remote-related states renewed
				conn.irs = segment.TcpHeader.SeqNum
				conn.expectedSeqNum.Store(segment.TcpHeader.SeqNum + 1)
				conn.recvBuf = NewRecvBuf(BUFFER_CAPACITY, segment.TcpHeader.SeqNum)
				return conn.doHandshakes(0, false)
			}

			// Handshake packet sent successfully
			conn.sndNxt.Add(1)
			conn.stateMachine(segment)
			return true
		}
	}
}

func (conn *VTCPConn) run() {
	go conn.handleRTO()
	for {
		segment := <-conn.recvChan
		conn.stateMachine(segment)
	}
}

// A wrapper around tcpstack.send() for packet containing data.
// Use conn.sendCTL() instead for packet without data
func (conn *VTCPConn) send(packet *proto.TCPPacket) error {
	err := send(conn.t, packet, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
	conn.resetRetransTimer(false)
	return err
}

// Send CTL Packet
func (conn *VTCPConn) sendCTL(seq uint32, ack uint32, flag uint8) (*proto.TCPPacket, error) {
	packet := proto.NewTCPacket(conn.LocalPort, conn.RemotePort, seq, ack, flag, make([]byte, 0), uint16(conn.windowSize.Load()))
	err := send(conn.t, packet, conn.LocalAddr, conn.RemoteAddr)
	if err != nil {
		return nil, err
	}
	return packet, nil
}

// Continuously send out new data in the send buffer
func (conn *VTCPConn) sendBufferedData() {
	b := conn.sendBuf
	for {
		// wait until there's new data to send
		conn.mu.Lock()
		for conn.numBytesNotSent() == 0 {
			conn.mu.Unlock()
			<-b.hasUnsentC
			conn.mu.Lock()
		}
		b.hasUnsentC = make(chan struct{}, b.capacity)
		conn.mu.Unlock()

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
			interval := float64(2)
			timeout := time.NewTimer(time.Duration(interval) * time.Second)
			<-timeout.C
			conn.mu.RLock()
			for conn.sndWnd.Load() == 0 && conn.sndUna.Load() == oldUna {
				conn.mu.RUnlock()
				err := send(conn.t, packet, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.RemoteAddr)
				if err != nil {
					logger.Error(err.Error())
					return
				}
				conn.sndNxt.Store(newNxt)
				logger.Debug("Sent zwp packet", "byte", string(byteToSend))

				timeout.Reset(time.Duration(interval) * time.Second)
				<-timeout.C
				conn.mu.RLock()
			}
			conn.mu.RUnlock()
		} else {
			numBytes, bytesToSend := conn.bytesNotSent(proto.MSS)
			if numBytes == 0 {
				logger.Debug("Window is not zero, but no sendable bytes")
				continue
			}
			packet := proto.NewTCPacket(conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemotePort,
				conn.sndNxt.Load(), conn.expectedSeqNum.Load(),
				header.TCPFlagAck, bytesToSend, uint16(conn.windowSize.Load()))
			err := conn.send(packet)
			if err != nil {
				logger.Error(err.Error())
				return
			}
			conn.inflightQ.PushBack(&packetMetadata{length: uint32(numBytes), packet: packet, timeSent: time.Now()}) // no need to lock the queue?
			logger.Debug("Pushed onto the queue", "SEQ", packet.TcpHeader.SeqNum, "len", numBytes)
			conn.sndNxt.Add(uint32(numBytes))
		}
	}
}
