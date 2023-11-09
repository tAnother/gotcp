package tcpstack

import (
	"fmt"
	"sync"
)

const BUFFER_CAPACITY = 1<<16 - 1

// We take this MIT licensed package as an example: https://github.com/smallnest/ringbuffer/blob/master/ring_buffer.go

// ------|----------------|--------------------|
//     head              tail               capacity
// head < capacity; tail < capacity

type CircBuff struct {
	buff     []byte
	capacity uint16
	head     uint16
	tail     uint16
	isFull   bool
	lock     sync.Mutex
}

func NewCircBuff(capacity uint16) *CircBuff {
	return &CircBuff{
		buff:     make([]byte, capacity),
		capacity: capacity,
	}
}

// Reads content on the circular buffer into the provided buffer with length len(buff)
func (cb *CircBuff) Read(buf []byte) (uint16, error) {
	if len(buf) == 0 {
		return 0, nil
	}

	// windowSize := cb.getWindowSize()
	cb.lock.Lock()
	defer cb.lock.Unlock()

	// 1. Base case: empty circular buff
	if cb.head == cb.tail && !cb.isFull {
		return 0, fmt.Errorf("buffer is empty")
	}

	// 2. if head tail are in order
	if cb.tail > cb.head {
		bytesToRead := cb.tail - cb.head
		if bytesToRead > uint16(len(buf)) {
			bytesToRead = uint16(len(buf))
		}
		copy(buf, cb.buff[cb.head:cb.head+bytesToRead])
		cb.head = (cb.head + bytesToRead) % cb.capacity
		return bytesToRead, nil
	}

	//3. if head and tail are in reversed order
	bytesToRead := cb.capacity - cb.head + cb.tail
	if bytesToRead > uint16(len(buf)) {
		bytesToRead = uint16(len(buf))
	}
	end := cb.head + bytesToRead
	if end <= cb.capacity {
		copy(buf, cb.buff[cb.head:end])
	} else {
		copy(buf, cb.buff[cb.head:cb.capacity])
		copy(buf[cb.capacity-cb.head:], cb.buff[:(end%cb.capacity)])
	}

	//4. update head and isFull
	cb.isFull = false
	cb.head = end % cb.capacity
	return bytesToRead, nil
}

func (cb *CircBuff) Write(buf []byte) (bytesWritten uint16, err error) {
	if len(buf) == 0 {
		return 0, nil
	}

	cb.lock.Lock()
	defer cb.lock.Unlock()

	//1. base case: circurlar buff is full, we cannot write
	if cb.isFull {
		return 0, fmt.Errorf("buffer is full")
	}

	//2. calculate free space
	var avail uint16
	if cb.tail >= cb.head {
		avail = cb.capacity - cb.tail + cb.head
	} else {
		avail = cb.head - cb.tail
	}

	// 3. check if there's a write overflow
	if len(buf) > int(avail) {
		err = fmt.Errorf("write overflow")
		buf = buf[:avail] //cut off the write buffer to available space
	}

	// 4. write to the circular buffer
	bytesWritten = uint16(len(buf))
	if cb.tail >= cb.head {
		tailSpace := cb.capacity - cb.tail
		if tailSpace >= (bytesWritten) { // if there's enough leftover space from tail ptr to the end
			copy(cb.buff[cb.tail:], buf)
			cb.tail += bytesWritten
		} else { // if not, need to copy two slices
			copy(cb.buff[cb.tail:], buf[:tailSpace])
			headSpace := bytesWritten - tailSpace
			copy(cb.buff[0:], buf[tailSpace:])
			cb.tail = headSpace
		}
	} else {
		copy(cb.buff[cb.tail:], buf)
		cb.tail += bytesWritten
	}

	// 5. check if circ buff  is full
	if cb.tail == cb.capacity {
		cb.isFull = true
	}
	if cb.tail == cb.head {
		cb.isFull = true
	}
	return bytesWritten, err
}

// Gets the available read bytes
func (cb *CircBuff) WindowSize() uint16 {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	if cb.head == cb.tail { //the buffer is full
		if cb.isFull {
			return cb.capacity
		}
		return 0
	}

	if cb.head > cb.tail {
		return cb.capacity - cb.head + cb.tail
	}
	return cb.tail - cb.head
}

// Gets the available write space
func (cb *CircBuff) FreeSpace() uint16 {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	if cb.head == cb.tail {
		if cb.isFull {
			return 0
		}
		return cb.capacity
	}

	if cb.tail < cb.head {
		return cb.head - cb.tail
	}
	return cb.capacity - cb.tail + cb.head
}

func (cb *CircBuff) IsEmpty() bool {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	return cb.head == cb.tail && !cb.isFull
}

func (cb *CircBuff) IsFull() bool {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	return cb.isFull
}

func (cb *CircBuff) Bytes() []byte {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	if cb.head == cb.tail {
		if cb.isFull {
			buf := make([]byte, cb.capacity)
			copy(buf, cb.buff[cb.head:])
			copy(buf[cb.capacity-cb.head:], cb.buff[:cb.tail])
			return buf
		}
		return nil
	}

	if cb.tail > cb.head {
		buf := make([]byte, cb.tail-cb.head)
		copy(buf, cb.buff[cb.head:cb.tail])
		return buf
	}

	n := cb.capacity - cb.head + cb.tail
	buf := make([]byte, n)

	if cb.head+n < cb.capacity {
		copy(buf, cb.buff[cb.head:cb.head+n])
	} else {
		headBytes := cb.capacity - cb.head
		copy(buf, cb.buff[cb.head:cb.capacity])
		tailBytes := n - headBytes
		copy(buf[headBytes:], cb.buff[:tailBytes])
	}
	return buf
}

func (cb *CircBuff) NextExpectedByte() uint16 {
	cb.lock.Lock()
	defer cb.lock.Unlock()
	return cb.tail + 1
}
