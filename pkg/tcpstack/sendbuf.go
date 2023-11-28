package tcpstack

type sendBuf struct {
	buf      []byte
	capacity uint // buffer length

	iss uint32 // init seq num
	lbw uint32 // last byte written

	hasUnsentC chan struct{} // signaling there are things to be sent
	freespaceC chan struct{} // signaling some bytes are acked
}

func newSendBuf(capacity uint, iss uint32) *sendBuf {
	return &sendBuf{
		buf:        make([]byte, capacity),
		capacity:   capacity,
		iss:        iss,
		lbw:        iss + 1, // to account for handshake
		hasUnsentC: make(chan struct{}, capacity),
		freespaceC: make(chan struct{}, capacity),
	}
}

func (b *sendBuf) index(num uint32) uint {
	return uint((num - b.iss)) % uint(b.capacity)
}

// Space left in the buffer.
// conn.mu should be locked on entry.
func (conn *VTCPConn) sendBufFreeSpace() uint {
	if uint(conn.sendBuf.lbw-conn.sndUna.Load()) <= conn.sendBuf.capacity {
		return conn.sendBuf.capacity - uint(conn.sendBuf.lbw-conn.sndUna.Load())
	}
	return 0
}

// Number of bytes in the buffer that's not yet sent to the receiver.
// conn.mu should be locked on entry.
func (conn *VTCPConn) numBytesNotSent() uint32 {
	return conn.sendBuf.lbw - conn.sndNxt.Load()
}

// Current max length to send to the receiver, i.e., the
// offered window less the amount of data sent but not acknowledged.
// [SND.NXT, SND.NXT+usable window] represents the sequence numbers
// that the remote (receiving) TCP endpoint is willing to receive.
// conn.mu should be locked on entry.
func (conn *VTCPConn) usableSendWindow() uint32 {
	if conn.sndUna.Load()+uint32(conn.sndWnd.Load()) > conn.sndNxt.Load() {
		return conn.sndUna.Load() + uint32(conn.sndWnd.Load()) - conn.sndNxt.Load()
	}
	return 0
}

// Write into the buffer, and return the number of bytes written.
// If the buffer is full, block until there is room to write data.
func (conn *VTCPConn) writeToSendBuf(data []byte) int {
	b := conn.sendBuf
	conn.mu.Lock()
	defer conn.mu.Unlock()

	for conn.sendBufFreeSpace() == 0 {
		conn.mu.Unlock()
		<-b.freespaceC
		conn.mu.Lock()
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

// Return an array of new bytes to send and its length (at max numBytes)
func (conn *VTCPConn) bytesNotSent(numBytes uint) (uint, []byte) {
	b := conn.sendBuf
	conn.mu.RLock()
	defer conn.mu.RUnlock()

	numBytes = min(numBytes, uint(conn.numBytesNotSent()), uint(conn.usableSendWindow()))
	if numBytes == 0 {
		// This is mainly used when usable send window = 0, where we need to stop sending new data
		// while keep retransmitting buf[SND.UNA, SND.UNA + SND.WND)
		// It can cause spin wait especially when SND.NXT & SND.WND both exceed SND.UNA too much
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
