package proto

import "sync"

type CircBuff struct {
	buff     []byte
	capacity uint32
	size     uint32
	head     uint32
	tail     uint32
	lock     sync.Mutex
	// segments
	//...TBD
}

func NewCircBuff(capacity uint32) *CircBuff {
	return &CircBuff{
		buff:     make([]byte, capacity),
		capacity: capacity,
		lock:     sync.Mutex{},
	}
}
