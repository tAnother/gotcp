package tcpstack

import (
	"fmt"
	"sync"
)

type sendBuf struct {
	buf []byte

	capacity int
	wnd      int
	iss      int // init seq num
	una      int // first byte unacked
	nxt      int // next byte to send
	nbw      int // next byte to write

	hasUnsentC chan struct{}  // signaling there are things to be sent
	freespaceC chan struct{}  // signaling some bytes are acked
	sentQueue  map[uint32]int // seq num : length  // TODO: this might get larger and larger, bad :(  clearing the map can also add some overhead

	mu *sync.Mutex
}

func newSendBuf(capacity, iss int) *sendBuf {
	return &sendBuf{
		buf:        make([]byte, capacity),
		capacity:   capacity,
		wnd:        capacity,
		iss:        iss,
		una:        iss,
		nxt:        iss,
		nbw:        iss,
		hasUnsentC: make(chan struct{}, 1),
		freespaceC: make(chan struct{}, 1),
		sentQueue:  make(map[uint32]int),
		mu:         &sync.Mutex{},
	}
}

func (b *sendBuf) index(num int) int {
	return (num - b.iss) % b.capacity
}

// Free space left in the buffer.
// The buffer should be locked on entry.
func (b *sendBuf) freeSpace() int {
	return b.una + b.wnd - b.nbw
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
	}
	b.freespaceC = make(chan struct{}, 1)

	nbwIdx := b.index(b.nbw)
	unaIdx := b.index(b.una)

	if nbwIdx < unaIdx || nbwIdx+len(data) <= b.capacity {
		copy(b.buf[nbwIdx:], data)
		b.nbw += len(data)
	} else { // need to wrap around
		firstHalf := b.capacity - nbwIdx
		copy(b.buf[nbwIdx:], data[:firstHalf])
		copy(b.buf[0:], data[firstHalf:])
		b.nbw = len(data) - firstHalf
	}

	b.hasUnsentC <- struct{}{}
	return len(data)
}

// Number of bytes to send.
// The buffer should be locked on entry.
func (b *sendBuf) numBytesUnsent() int {
	return b.nbw - b.nxt
}

// Return an array of bytes to send, starting from seqNum to (at max) seqNum + numBytes.
// If there are no bytes to send, block until there are.
//
// TODO: check if it is specified in the protocol how many bytes should be sent each time
func (b *sendBuf) send(seqNum uint32, numBytes int) []byte {
	if numBytes == 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.numBytesUnsent() == 0 {
		b.mu.Unlock()
		<-b.hasUnsentC
		b.mu.Lock()
	}
	b.hasUnsentC = make(chan struct{}, 1)

	numBytes = min(numBytes, b.numBytesUnsent())
	ret := make([]byte, numBytes)

	nxtIdx := b.index(b.nxt)
	nbwIdx := b.index(b.nbw)

	if nxtIdx < nbwIdx {
		copy(ret, b.buf[nxtIdx:nxtIdx+numBytes])
	} else {
		firstHalf := b.wnd - nxtIdx
		copy(ret[:firstHalf], b.buf[nxtIdx:])
		copy(ret[firstHalf:], b.buf[:numBytes-firstHalf])
	}
	b.nxt += numBytes
	b.sentQueue[seqNum] = numBytes

	return ret
}

// Mark seqNum as acked.
func (b *sendBuf) ack(seqNum uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	length, ok := b.sentQueue[seqNum]
	if !ok {
		return fmt.Errorf("cannot find seq num %v in the sent queue", seqNum)
	}

	b.una = max(b.una, int(seqNum)+length)
	b.freespaceC <- struct{}{}
	if b.una == b.nxt {
		clear(b.sentQueue)
	}
	return nil
}
