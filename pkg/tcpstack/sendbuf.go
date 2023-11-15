package tcpstack

import (
	"sync"
)

// TODO: tcp seq num wrap around

type sendBuf struct {
	buf      []byte
	capacity uint // buffer length

	iss uint32 // init seq num
	lbw uint32 // last byte written

	hasUnsentC chan struct{} // signaling there are things to be sent
	freespaceC chan struct{} // signaling some bytes are acked

	mu *sync.Mutex
}

func newSendBuf(capacity uint, iss uint32) *sendBuf {
	return &sendBuf{
		buf:        make([]byte, capacity),
		capacity:   capacity,
		iss:        iss,
		lbw:        iss + 1, // to account for handshake // TODO: might need to change when SYN packet is dropped
		hasUnsentC: make(chan struct{}, 1),
		freespaceC: make(chan struct{}, 1),
		mu:         &sync.Mutex{},
	}
}

func (b *sendBuf) index(num uint32) uint {
	return uint((num - b.iss)) % uint(b.capacity)
}

// Space left in the buffer.
// The buffer should be locked on entry.
func (conn *VTCPConn) sendBufFreeSpace() uint {
	return conn.sendBuf.capacity - uint(conn.sendBuf.lbw) - uint(conn.sndUna.Load())
}

// Number of send-able bytes
// The buffer should be locked on entry.
func (conn *VTCPConn) numBytesNotSent() uint32 {
	return conn.sendBuf.lbw - conn.sndNxt.Load()
}

// Max number to send to the receiver
func (conn *VTCPConn) usableSendWindow() uint32 {
	logger.Println("snduna: ", conn.sndUna.Load())
	logger.Println("sndWnd: ", conn.sndWnd.Load())
	logger.Println("sndNxt: ", conn.sndNxt.Load())

	return conn.sndUna.Load() + uint32(conn.sndWnd.Load()) - conn.sndNxt.Load()
}

// Write into the buffer.
// If the buffer is full, block until there is enough room to write all data.
func (conn *VTCPConn) write(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	b := conn.sendBuf
	b.mu.Lock()
	defer b.mu.Unlock()

	for conn.sendBufFreeSpace() < uint(len(data)) {
		b.mu.Unlock()
		<-b.freespaceC
		b.mu.Lock()
	}
	b.freespaceC = make(chan struct{}, 1)

	lbwIdx := b.index(b.lbw)
	unaIdx := b.index(conn.sndUna.Load())

	if lbwIdx < unaIdx || lbwIdx+uint(len(data)) <= b.capacity {
		copy(b.buf[lbwIdx:], data)
		b.lbw += uint32(len(data))
	} else { // need to wrap around
		firstHalf := int(b.capacity) - int(lbwIdx)
		copy(b.buf[lbwIdx:], data[:firstHalf])
		copy(b.buf[0:], data[firstHalf:])
		b.lbw = uint32(len(data) - firstHalf)
	}

	b.hasUnsentC <- struct{}{}
	return len(data)
}

// Return the segment corresponding to seqNum. Use for retransmission
func (b *sendBuf) getBytes(seqNum uint32, length int) []byte {
	if length == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	start := b.index(seqNum)
	end := b.index(seqNum + uint32(length))

	if start < end {
		return b.buf[start:end]
	}
	ret := make([]byte, length)

	copy(ret, b.buf[start:])
	copy(ret[b.capacity-start:], b.buf[:end])
	return ret
}

// Return an array of new bytes to send and its length (at max numBytes)
// If there are no bytes to send, block until there are.
func (conn *VTCPConn) bytesNotSent(numBytes uint) (uint, []byte) {
	if numBytes == 0 {
		return 0, nil
	}
	b := conn.sendBuf
	b.mu.Lock()
	defer b.mu.Unlock()

	for conn.numBytesNotSent() == 0 {
		b.mu.Unlock()
		<-b.hasUnsentC
		b.mu.Lock()
	}
	b.hasUnsentC = make(chan struct{}, 1)

	logger.Println("numbytes: ", numBytes)
	logger.Println("numBytesNotSent: ", uint(conn.numBytesNotSent()))
	logger.Println("usableSendWindow: ", uint(conn.usableSendWindow()))
	numBytes = min(numBytes, uint(conn.numBytesNotSent()), uint(conn.usableSendWindow()))
	logger.Println("min-numbytes: ", numBytes)

	ret := make([]byte, numBytes)

	nxtIdx := b.index(conn.sndNxt.Load())
	lbwIdx := b.index(b.lbw)

	if nxtIdx < lbwIdx {
		copy(ret, b.buf[nxtIdx:nxtIdx+numBytes])
	} else {
		firstHalf := uint(b.lbw) - nxtIdx
		copy(ret[:firstHalf], b.buf[nxtIdx:])
		copy(ret[firstHalf:], b.buf[:numBytes-firstHalf])
	}

	return numBytes, ret
}

// Mark seqNum as acked
func (conn *VTCPConn) ack(seqNum uint32) error {
	if conn.sndUna.Load() < seqNum {
		conn.sndUna.Store(seqNum)
		conn.sendBuf.mu.Lock()
		conn.sendBuf.freespaceC <- struct{}{}
		conn.sendBuf.mu.Unlock()
	}
	return nil
}
