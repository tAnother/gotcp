package tcpstack

import (
	"fmt"
	"io"
	"iptcp-nora-yu/pkg/repl"
	"net/netip"
	"strconv"
	"strings"
)

func TcpRepl(t *TCPGlobalInfo) *repl.REPL {
	r := repl.NewRepl()
	r.AddCommand("a", acceptHandler(t), "Listens and accpets a port. usage: a <port>")
	r.AddCommand("ls", lsHandler(t), "Lists all sockets. usage: ls")
	r.AddCommand("c", connectHandler(t), "Creates a new socket that connects to the specified virtual IP address and port. usage: c <vip> <port>")
	r.AddCommand("cl", closeHandler(t), "Closes a socket. usage: cl <socket ID>")
	r.AddCommand("r", readBytesHandler(t), "Read n bytes of data on a socket. usage: r <socket ID> <numbytes>")
	r.AddCommand("s", sendBytesHandler(t), "Send n bytes of data on a socket. usage: r <socket ID> <bytes to send>")
	return r
}

func acceptHandler(t *TCPGlobalInfo) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 2 {
			return fmt.Errorf("usage: a <port>")
		}
		port, err := strconv.Atoi(args[1])
		if err != nil {
			return err
		}
		if !IsUint16(port) {
			return fmt.Errorf("input %v is out of range", port)
		}

		l, err := VListen(t, uint16(port))
		if err != nil {
			return err
		}

		go func() {
			for {
				_, err := l.VAccept()
				if err != nil {
					io.WriteString(config.Writer, fmt.Sprintln(err))
				}
			}
		}()
		return nil
	}
}

func connectHandler(t *TCPGlobalInfo) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 3 {
			return fmt.Errorf("usage: c <vip> <port>")
		}
		port, err := strconv.Atoi(args[2])
		if err != nil {
			return err
		}
		if !IsUint16(port) {
			return fmt.Errorf("input %v is out of range", port)
		}

		_, err = VConnect(t, netip.MustParseAddr(args[1]), uint16(port))
		return err
	}
}

func lsHandler(t *TCPGlobalInfo) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 1 {
			return fmt.Errorf("usage: ls")
		}

		sockets := t.GetSocketTableString()

		_, err := io.WriteString(config.Writer, "SID\tLAddr\t\tLPort\tRAddr\t\tRPort\tStatus\n")
		if err != nil {
			return err
		}

		for _, socketsInfo := range sockets {
			_, err := io.WriteString(config.Writer, socketsInfo)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func closeHandler(t *TCPGlobalInfo) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 2 {
			return fmt.Errorf("usage: cl <socket id>")
		}
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("<socket ID> has to be an integer")
		}
		l := t.findListenerSocket(int32(id))
		if l != nil {
			return l.VClose()
		}
		conn := t.findNormalSocket(int32(id))
		if conn != nil {
			return conn.VClose()
		}
		return fmt.Errorf("socket id %v not found", id)
	}
}

func readBytesHandler(t *TCPGlobalInfo) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 3 {
			return fmt.Errorf("usage: r <socket ID> <numbytes>")
		}
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("<socket ID> has to be an integer")
		}

		numBytes, err := strconv.Atoi(args[2])
		if err != nil || numBytes < 0 {
			return fmt.Errorf("<numbytes> has to be a non-negative integer")
		}

		conn := t.findNormalSocket(int32(id))
		if conn == nil {
			return fmt.Errorf("cannot find socket %v", id)
		}

		buffer := make([]byte, numBytes)
		bytesRead, err := conn.VRead(buffer)
		if err != nil {
			return err
		}
		io.WriteString(config.Writer, fmt.Sprintf("Read %v bytes: %v\n", bytesRead, string(buffer)))
		return nil
	}
}

func sendBytesHandler(t *TCPGlobalInfo) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 3 {
			return fmt.Errorf("usage: r <socket ID> <numbytes>")
		}
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("<socket ID> has to be an integer")
		}
		conn := t.findNormalSocket(int32(id))
		if conn == nil {
			return fmt.Errorf("cannot find socket %v", id)
		}

		data := args[2]
		bytesWritten, err := conn.VWrite([]byte(data))
		if err != nil {
			return err
		}
		io.WriteString(config.Writer, fmt.Sprintf("Wrote %v bytes\n", bytesWritten))
		return nil
	}
}
