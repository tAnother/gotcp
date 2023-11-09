package tcpstack

import (
	"fmt"
	"sync"
)

type sendBuf struct {
	buf []byte

	capacity         int
	firstByteUnacked int // pointer or index into the buf
	nextByteToSend   int // pointer or index into the buf
	nextByteToWrite  int // pointer or index into the buf

	isFull     bool
	hasUnsent  bool
	hasUnsentC chan struct{}    // signaling there are things to be sent
	freespaceC chan struct{}    // signaling some bytes are acked
	sentQueue  map[uint32]index // seq num : segment index in buffer  // TODO: this gets larger and larger, bad :(

	mu *sync.Mutex
}

type index struct {
	start int
	end   int
}

func newSendBuf(capacity int) *sendBuf {
	return &sendBuf{
		buf:        make([]byte, capacity),
		capacity:   capacity,
		sentQueue:  make(map[uint32]index),
		hasUnsentC: make(chan struct{}, 1),
		freespaceC: make(chan struct{}, 1),
		mu:         &sync.Mutex{},
	}
}

// Free space left in the buffer.
// The buffer should be locked on entry.
func (b *sendBuf) freeSpace() int {
	if b.isFull {
		return 0
	}
	if b.firstByteUnacked == b.nextByteToWrite {
		return b.capacity
	}
	if b.firstByteUnacked < b.nextByteToWrite {
		return b.capacity - (b.nextByteToWrite - b.firstByteUnacked)
	}
	return b.firstByteUnacked - b.nextByteToWrite
}

// Write into the buffer.
// If the buffer is full, block until there is enough room to write all data.
func (b *sendBuf) write(data []byte) int {
	if len(data) == 0 {
		return 0
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for b.freeSpace() < len(data) {
		b.mu.Unlock()
		<-b.freespaceC
		b.mu.Lock()
		b.freespaceC = make(chan struct{}, 1)
	}

	if b.nextByteToWrite < b.firstByteUnacked ||
		b.nextByteToWrite+len(data) <= b.capacity {
		copy(b.buf[b.nextByteToWrite:], data)
		b.nextByteToWrite += len(data)
	} else { // need to wrap around
		firstHalf := b.capacity - b.nextByteToWrite
		copy(b.buf[b.nextByteToWrite:], data[:firstHalf])
		copy(b.buf[0:], data[firstHalf:])
		b.nextByteToWrite = len(data) - firstHalf
	}

	b.isFull = b.nextByteToWrite == b.firstByteUnacked
	if !b.hasUnsent {
		b.hasUnsent = true
		b.hasUnsentC <- struct{}{}
	}
	return len(data)
}

// Number of bytes to send.
// The buffer should be locked on entry.
func (b *sendBuf) numBytesUnsent() int {
	if b.nextByteToSend == b.nextByteToWrite {
		if !b.hasUnsent {
			return 0
		}
		return b.capacity
	}
	if b.nextByteToSend < b.nextByteToWrite {
		return b.nextByteToWrite - b.nextByteToSend
	}
	return b.capacity - b.nextByteToSend + b.nextByteToWrite
}

// Return an array of bytes to send. If there are no bytes to send, block until there are.
//
// TODO: check if it is specified in the protocol how many bytes should be sent each time
// right now it sends as much as possible (but no larger than MSS)
func (b *sendBuf) send(seqNum uint32, numBytes int) []byte {
	if numBytes == 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.hasUnsent {
		b.mu.Unlock()
		<-b.hasUnsentC
		b.mu.Lock()
		// b.hasUnsentC = make(chan struct{}, 1)
	}

	numBytes = min(numBytes, b.numBytesUnsent())
	ret := make([]byte, numBytes)

	if b.nextByteToSend < b.nextByteToWrite {
		copy(ret, b.buf[b.nextByteToSend:b.nextByteToSend+numBytes])
		b.sentQueue[seqNum] = index{b.nextByteToSend, b.nextByteToSend + numBytes}
		b.nextByteToSend += numBytes
	} else {
		firstHalf := b.capacity - b.nextByteToSend
		copy(ret[:firstHalf], b.buf[b.nextByteToSend:])
		copy(ret[firstHalf:], b.buf[:numBytes-firstHalf])
		b.sentQueue[seqNum] = index{b.nextByteToSend, numBytes - firstHalf}
		b.nextByteToSend = numBytes - firstHalf
	}

	b.hasUnsent = b.nextByteToSend != b.nextByteToWrite
	return ret
}

// Mark seqNum as acked
//
// This method is UNSAFE. The sentQueue contains outdated data;
// the passed in seqNum must be monotonically increasing
func (b *sendBuf) ack(seqNum uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	idx, ok := b.sentQueue[seqNum]
	if !ok {
		// logger.Printf("cannot find seq num %v in the sent queue\n", seqNum)
		return fmt.Errorf("cannot find seq num %v in the sent queue", seqNum)
	}
	delete(b.sentQueue, seqNum)

	if idx.start != idx.end || b.isFull { // not a zero-length segment
		b.isFull = false
		b.freespaceC <- struct{}{}
	}
	b.firstByteUnacked = idx.end
	return nil
}
