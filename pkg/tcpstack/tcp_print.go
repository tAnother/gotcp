package tcpstack

import "fmt"

func GetSocketTableString() []string {
	tcb.TableMu.RLock()
	defer tcb.TableMu.RUnlock()
	size := len(tcb.ListenerTable) + len(tcb.ConnTable)
	res := make([]string, size)
	index := 0
	for _, l := range tcb.ListenerTable {
		res[index] = fmt.Sprintf("%v\t%v\t%v\t%v\t%v\t%v\n", l.socketId, "0.0.0.0\t", l.port, "0.0.0.0\t", "0\t", "LISTEN")
		index += 1
	}

	for _, conn := range tcb.ConnTable {
		// conn.stateMu.RLock()
		res[index] = fmt.Sprintf("%v\t%v\t%v\t%v\t%v\t%v\n", conn.socketId, conn.localAddr, conn.localPort, conn.remoteAddr, conn.remotePort, conn.state)
		// conn.stateMu.RUnlock()
		index += 1
	}
	return res
}
