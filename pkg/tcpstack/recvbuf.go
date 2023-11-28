package tcpstack

import (
	"fmt"
	"io"
	"sync"
)

const BUFFER_CAPACITY = 1<<16 - 1

// const BUFFER_CAPACITY = 8

// We take this MIT licensed package as an example: https://github.com/smallnest/ringbuffer/blob/master/ring_buffer.go

// ------|-----------------|--1byte--|--------------------|
//     lbr+++++++++++++++++|--1byte--nxt               capacity
// lbr can start at any point. The init value should be the init seq num

type recvBuf struct {
	buff     []byte
	capacity uint32
	lbr      uint32
	nxt      uint32
	isFull   bool
	finRecvd bool
	canRead  chan struct{}
	lock     sync.Mutex
}

func NewRecvBuf(capacity uint32, start uint32) *recvBuf {
	return &recvBuf{
		buff:     make([]byte, capacity),
		capacity: capacity,
		lbr:      start,
		nxt:      start + 1,
		canRead:  make(chan struct{}, capacity),
	}
}

// Reads content on the circular buffer into the provided buffer with length len(buff)
func (cb *recvBuf) Read(buf []byte) (bytesRead uint32, err error) {
	if len(buf) == 0 {
		return 0, nil
	}

	cb.lock.Lock()
	defer cb.lock.Unlock()

	if cb.finRecvd && cb.nxt-1 == cb.lbr {
		return 0, io.EOF
	}

	// 1. Base case: empty circular buff
	if ((cb.nxt-1)%cb.capacity == cb.lbr%cb.capacity) && !cb.isFull {
		cb.lock.Unlock()
		<-cb.canRead
		cb.lock.Lock()
	}
	head := cb.lbr % cb.capacity
	tail := (cb.nxt - 1) % cb.capacity

	// 2. if head tail are in order
	if tail > head {
		bytesRead = tail - head
		if bytesRead > uint32(len(buf)) {
			bytesRead = uint32(len(buf))
		}
		copy(buf, cb.buff[head:head+bytesRead])
	} else { //3. if head and tail are in reversed order
		bytesRead = cb.capacity - head + tail
		if bytesRead > uint32(len(buf)) {
			bytesRead = uint32(len(buf))
		}
		end := head + bytesRead
		if end <= cb.capacity {
			copy(buf, cb.buff[head:end])
		} else {
			copy(buf, cb.buff[head:cb.capacity])
			copy(buf[cb.capacity-head:], cb.buff[:(end%cb.capacity)])
		}

	}
	// 4. update lbr and isFull
	cb.isFull = false
	cb.lbr = cb.lbr + bytesRead
	cb.canRead = make(chan struct{}, cb.capacity)
	return bytesRead, nil
}

func (cb *recvBuf) Write(buf []byte) (bytesWritten uint32, err error) {
	if len(buf) == 0 {
		return 0, nil
	}

	cb.lock.Lock()
	defer cb.lock.Unlock()

	head := cb.lbr % cb.capacity
	tail := (cb.nxt - 1) % cb.capacity

	//1. base case: circurlar buff is full, we cannot write
	if cb.isFull {
		return 0, fmt.Errorf("buffer is full")
	}

	var avail uint32
	//3. calculate free space
	if tail >= head {
		avail = cb.capacity - tail + head
	} else {
		avail = head - tail
	}

	// 2. check if there's no overflow. This cannot happen since the incoming data is trimmed to fit inside the window
	if len(buf) > int(avail) {
		logger.Debug("write overflow")
		buf = buf[:avail] //cut off the write buffer to available space
	}

	// 4. write to the circular buffer
	bytesWritten = uint32(len(buf))
	if tail >= head {
		tailSpace := cb.capacity - tail
		if tailSpace >= (bytesWritten) { // if there's enough leftover space from tail to the end
			copy(cb.buff[tail:], buf)
		} else { // if not, need to copy two slices
			copy(cb.buff[tail:], buf[:tailSpace])
			copy(cb.buff[0:], buf[tailSpace:])
		}
	} else {
		copy(cb.buff[tail:], buf)
	}

	//update nxt byte expected
	cb.nxt += bytesWritten
	cb.canRead <- struct{}{} //signal the waiting read process

	// 5. check if circ buff is full
	if (cb.nxt-1-cb.lbr)%cb.capacity == 0 {
		cb.isFull = true
	}
	return bytesWritten, err
}

// Gets the available read bytes
func (cb *recvBuf) WindowSize() uint32 {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	head := cb.lbr % cb.capacity
	tail := (cb.nxt - 1) % cb.capacity
	if head == tail { //the buffer is full
		if cb.isFull {
			return cb.capacity
		}
		return 0
	}

	if head > tail {
		return cb.capacity - head + tail
	}
	return tail - head
}

// Gets the available write space
func (cb *recvBuf) FreeSpace() uint32 {
	cb.lock.Lock()
	defer cb.lock.Unlock()
	head := cb.lbr % cb.capacity
	tail := (cb.nxt - 1) % cb.capacity
	if head == tail {
		if cb.isFull {
			return 0
		}
		return cb.capacity
	}

	if tail < head {
		return head - tail
	}
	return cb.capacity - tail + head
}

func (cb *recvBuf) IsEmpty() bool {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	head := cb.lbr % cb.capacity
	tail := cb.nxt%cb.capacity - 1
	return head == tail && !cb.isFull
}

func (cb *recvBuf) IsFull() bool {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	return cb.isFull
}

func (cb *recvBuf) Bytes() []byte {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	head := cb.lbr % cb.capacity
	tail := (cb.nxt - 1) % cb.capacity

	if head == tail {
		if cb.isFull {
			buf := make([]byte, cb.capacity)
			copy(buf, cb.buff[head:])
			copy(buf[cb.capacity-head:], cb.buff[:tail])
			return buf
		}
		return nil
	}

	if tail > head {
		buf := make([]byte, tail-head)
		copy(buf, cb.buff[head:tail])
		return buf
	}

	n := cb.capacity - head + tail
	buf := make([]byte, n)

	if head+n < cb.capacity {
		copy(buf, cb.buff[head:head+n])
	} else {
		headBytes := cb.capacity - head
		copy(buf, cb.buff[head:cb.capacity])
		tailBytes := n - headBytes
		copy(buf[headBytes:], cb.buff[:tailBytes])
	}
	return buf
}

func (cb *recvBuf) NextExpectedByte() uint32 {
	cb.lock.Lock()
	defer cb.lock.Unlock()
	return cb.nxt
}

func (cb *recvBuf) LastByteRead() uint32 {
	cb.lock.Lock()
	defer cb.lock.Unlock()
	return cb.lbr
}

func (cb *recvBuf) AdvanceNxt(seq uint32, isFin bool) {
	cb.lock.Lock()
	defer cb.lock.Unlock()
	cb.finRecvd = isFin
	cb.nxt = seq + 1
}

func (cb *recvBuf) SetLBR(seq uint32) {
	cb.lock.Lock()
	defer cb.lock.Unlock()
	cb.lbr = seq
}
