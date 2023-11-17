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
		hasUnsentC: make(chan struct{}, capacity),
		freespaceC: make(chan struct{}, capacity),
		mu:         &sync.Mutex{},
	}
}

func (b *sendBuf) index(num uint32) uint {
	return uint((num - b.iss)) % uint(b.capacity)
}

// Space left in the buffer.
// The buffer should be locked on entry.
func (conn *VTCPConn) sendBufFreeSpace() uint {
	if uint(conn.sendBuf.lbw-conn.sndUna.Load()) <= conn.sendBuf.capacity {
		return conn.sendBuf.capacity - uint(conn.sendBuf.lbw-conn.sndUna.Load())
	}
	return 0
}

// Number of bytes in the buffer that's not yet sent to the receiver.
// The buffer should be locked on entry.
func (conn *VTCPConn) numBytesNotSent() uint32 {
	return conn.sendBuf.lbw - conn.sndNxt.Load()
}

// Current max length to send to the receiver,
// i.e., the offered window less the amount of data sent but
// not acknowledged. [SND.NXT, SND.NXT+usable window] represents
// the sequence numbers that the remote (receiving) TCP endpoint
// is willing to receive.
func (conn *VTCPConn) usableSendWindow() uint32 {
	if conn.sndUna.Load()+uint32(conn.sndWnd.Load()) > conn.sndNxt.Load() {
		return conn.sndUna.Load() + uint32(conn.sndWnd.Load()) - conn.sndNxt.Load()
	}
	return 0
}

// Write into the buffer, and return the number of bytes written.
// If the buffer is full, block until there is room to write data.
func (conn *VTCPConn) write(data []byte) int {
	b := conn.sendBuf
	b.mu.Lock()
	defer b.mu.Unlock()

	for conn.sendBufFreeSpace() == 0 {
		b.mu.Unlock()
		<-b.freespaceC
		b.mu.Lock()
	}
	b.freespaceC = make(chan struct{}, b.capacity)

	numBytes := min(conn.sendBufFreeSpace(), uint(len(data)))
	lbwIdx := b.index(b.lbw)

	if lbwIdx+numBytes <= b.capacity {
		copy(b.buf[lbwIdx:], data[:numBytes])
	} else { // need to wrap around
		firstHalf := int(b.capacity) - int(lbwIdx)
		copy(b.buf[lbwIdx:], data[:firstHalf])
		copy(b.buf[0:], data[firstHalf:numBytes])
	}

	b.lbw += uint32(numBytes)
	b.hasUnsentC <- struct{}{}
	return int(numBytes)
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
func (conn *VTCPConn) bytesNotSent(numBytes uint, forProbe bool) (uint, []byte) {
	b := conn.sendBuf
	b.mu.Lock()
	defer b.mu.Unlock()

	for conn.numBytesNotSent() == 0 {
		b.mu.Unlock()
		<-b.hasUnsentC
		b.mu.Lock()
	}
	b.hasUnsentC = make(chan struct{}, b.capacity)

	if forProbe {
		nxt := b.buf[b.index(conn.sndNxt.Load())]
		return 1, []byte{nxt}
	}

	numBytes = min(numBytes, uint(conn.numBytesNotSent()), uint(conn.usableSendWindow()))
	if numBytes == 0 {
		// TODO: this is mainly used when usable send window = 0, where we need to stop sending new data
		// while keep retransmitting buf[SND.UNA, SND.UNA + SND.WND)
		// it can cause spin wait especially when SND.NXT & SND.WND both exceed SND.UNA too much
		// (probably when an early segment gets dropped?)
		// but blocking until usable send window becomes >0 would require observing SND.UNA & SND.WND
		return 0, nil
	}

	ret := make([]byte, numBytes)
	nxtIdx := b.index(conn.sndNxt.Load())

	if nxtIdx+numBytes <= b.capacity {
		copy(ret, b.buf[nxtIdx:nxtIdx+numBytes])
	} else {
		firstHalf := b.capacity - nxtIdx
		copy(ret[:firstHalf], b.buf[nxtIdx:])
		copy(ret[firstHalf:], b.buf[:numBytes-firstHalf])
	}
	return numBytes, ret
}
