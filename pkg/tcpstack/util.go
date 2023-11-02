package tcpstack

import (
	"math/rand"
	"time"
)

const (
	MAX_UINT16      = 65535
	MIN_RANDOM_PORT = 20000
)

func IsUint16(num int) bool {
	return num >= 0 && num <= MAX_UINT16
}

func generateStartSeqNum() uint32 {
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	return rand.Uint32()
}
