package tcpstack

import (
	"math/rand"
	"time"
)

const (
	MAX_UINT16      = 65535
	MIN_RANDOM_PORT = 20000
	MSL             = 3 * time.Second
	MIN_RTO         = 100  //100 ms
	MAX_RTO         = 5000 // 5 s
	ALPHA           = 0.8
	BETA            = 1.6
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
