package tcpstack

import (
	"math/rand"
	"time"
)

func isPortInUse(portKey uint16) bool {
	tcb.TableMu.RLock()
	defer tcb.TableMu.RUnlock()

	if _, ok := tcb.ListenerTable[portKey]; ok {
		return true
	}
	return false
}

func socketExists(endpoint TCPEndpointID) bool {
	tcb.TableMu.RLock()
	defer tcb.TableMu.RUnlock()
	if _, ok := tcb.ConnTable[endpoint]; ok {
		return true
	}
	return false
}

func bindListener(port uint16, l *VTCPListener) {
	tcb.TableMu.Lock()
	defer tcb.TableMu.Unlock()
	tcb.ListenerTable[port] = l
}

func bindSocket(endpoint TCPEndpointID, s *VTCPConn) {
	tcb.TableMu.Lock()
	defer tcb.TableMu.Unlock()
	tcb.ConnTable[endpoint] = s
}

func deleteSocket(endpoint TCPEndpointID) {
	tcb.TableMu.Lock()
	defer tcb.TableMu.Unlock()
	delete(tcb.ConnTable, endpoint)
}

// Generates an available (unused) port num
// Should have a better way to do this...
func generateRandomPortNum() uint16 {
	usedPorts := getUsedPorts()
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	for {
		randomNumber := uint16(MIN_RANDOM_PORT + rand.Intn((MAX_UINT16 - MIN_RANDOM_PORT)))
		found := false

		// Check if the random number is in the array
		for _, num := range usedPorts {
			if num == randomNumber {
				found = true
				break
			}
		}
		if !found {
			return randomNumber
		}
	}
}

func getUsedPorts() []uint16 {
	tcb.TableMu.RLock()
	defer tcb.TableMu.RUnlock()
	size := len(tcb.ConnTable) + len(tcb.ListenerTable)
	ports := make([]uint16, size)
	index := 0
	for endpoint := range tcb.ConnTable {
		ports[index] = endpoint.localPort
		index += 1
	}
	for port := range tcb.ListenerTable {
		ports[index] = port
	}
	return ports
}
