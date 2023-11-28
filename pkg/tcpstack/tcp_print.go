package tcpstack

import "fmt"

func (t *TCPGlobalInfo) GetSocketTableString() []string {
	t.tableMu.RLock()
	defer t.tableMu.RUnlock()
	size := len(t.listenerTable) + len(t.connTable)
	res := make([]string, size)
	index := 0
	for _, l := range t.listenerTable {
		res[index] = fmt.Sprintf("%v\t%v\t%v\t%v\t%v\t%v\n", l.socketId, "0.0.0.0\t", l.port, "0.0.0.0\t", "0", "LISTEN")
		index += 1
	}

	for _, conn := range t.connTable {
		res[index] = fmt.Sprintf("%v\t%v\t%v\t%v\t%v\t%v\n", conn.socketId, conn.TCPEndpointID.LocalAddr, conn.TCPEndpointID.LocalPort, conn.TCPEndpointID.RemoteAddr, conn.TCPEndpointID.RemotePort, stateString[conn.getState()])
		index += 1
	}
	return res
}
