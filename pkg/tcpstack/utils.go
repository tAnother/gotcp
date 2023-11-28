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

// Generates an available (unused) port num
func generateRandomPortNum(t *TCPGlobalInfo) uint16 {
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	for {
		port := uint16(MIN_RANDOM_PORT + rand.Intn((MAX_UINT16 - MIN_RANDOM_PORT)))

		// Check if the port number is in use
		if t.isPortInUse(port) {
			continue
		}

		return port
	}
}
