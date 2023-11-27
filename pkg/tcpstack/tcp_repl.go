package tcpstack

import (
	"bufio"
	"fmt"
	"io"
	"iptcp-nora-yu/pkg/proto"
	"iptcp-nora-yu/pkg/repl"
	"net/netip"
	"os"
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
	r.AddCommand("s", sendBytesHandler(t), "Send n bytes of data on a socket. usage: s <socket ID> <bytes to send>")
	r.AddCommand("sf", sendFileHandler(t), "Send file to destination addr port. usage: sf <file path> <dst addr> <dst port>")
	r.AddCommand("rf", readFileHandler(t), "Read a file into the destination file on a listening port. usage: rf <dest file> <port>")
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
			err := l.VClose()
			if err != nil {
				return err
			}
			io.WriteString(config.Writer, fmt.Sprintf("Socket %v closed\n", l.socketId))
			return nil
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
			return fmt.Errorf("usage: s <socket ID> <numbytes>")
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

func sendFileHandler(t *TCPGlobalInfo) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 4 {
			return fmt.Errorf("usage: sf <file path> <dst addr> <dst port>")
		}
		file, err := os.Open(args[1])
		if err != nil {
			return err
		}
		addr, err := netip.ParseAddr(args[2])
		if err != nil {
			return err
		}
		port, err := strconv.Atoi(args[3])
		if err != nil {
			return err
		}
		if !IsUint16(port) {
			return fmt.Errorf("input %v is out of range", port)
		}
		conn, err := VConnect(t, addr, uint16(port))
		if err != nil {
			return err
		}

		go func() {
			defer file.Close()
			reader := bufio.NewReaderSize(file, proto.MSS)
			buf := make([]byte, proto.MSS)
			bytesWritten := 0
			for {
				b, err := reader.Read(buf)
				if err == io.EOF {
					// Done sending. Close the connection
					err = conn.VClose()
					if err != nil {
						io.WriteString(config.Writer, fmt.Sprintf("failed to close the socket %v: %v\n", conn.socketId, err))
					}
					io.WriteString(config.Writer, fmt.Sprintf("Sent %v bytes\n", bytesWritten))
					return
				}
				if err != nil {
					err = conn.VClose()
					if err != nil {
						io.WriteString(config.Writer, fmt.Sprintf("failed to close the socket %v: %v\n", conn.socketId, err))
					}
					io.WriteString(config.Writer, fmt.Sprintf("error reading from file: %v\n", err))
					return
				}

				w, err := conn.VWrite(buf[:b])
				if err != nil {
					io.WriteString(config.Writer, fmt.Sprintf("error sending file: %v\n", err))
					return
				}
				bytesWritten += w
			}
		}()
		return nil
	}
}

func readFileHandler(t *TCPGlobalInfo) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 3 {
			return fmt.Errorf("usage: rf <dest file> <port>")
		}
		f, err := os.OpenFile(args[1], os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return fmt.Errorf("rf error: failed to open the file")
		}
		port, err := strconv.Atoi(args[2])
		if err != nil {
			f.Close()
			return fmt.Errorf("rf error: <port> has to be an integer")
		}

		l, err := VListen(t, uint16(port))
		if err != nil {
			f.Close()
			return fmt.Errorf("rf error: %v", err)
		}

		go func() {
			conn, err := l.VAccept()
			if err != nil {
				l.t.deleteListener(l.port)
				io.WriteString(config.Writer, fmt.Sprintln(err))
				f.Close()
				return
			}

			defer func() {
				f.Close()
				//Done Receiving. Close the connection
				err = l.VClose()
				if err != nil {
					io.WriteString(config.Writer, fmt.Sprintf("failed to close the listener socket %v: %v\n", l.socketId, err))
				}
				err = conn.VClose()
				if err != nil {
					io.WriteString(config.Writer, fmt.Sprintf("failed to close the socket %v: %v\n", conn.socketId, err))
				}
			}()

			io.WriteString(config.Writer, fmt.Sprintln("rf: client connected!"))
			totalBytesRead := uint32(0)
			for { //read until the sender closes the connection
				buf := make([]byte, proto.MSS)
				bytesRead, err := conn.recvBuf.Read(buf)
				conn.windowSize.Add(int32(bytesRead))
				f.Write(buf[:bytesRead])
				totalBytesRead += bytesRead
				if err == io.EOF {
					//get rid of the null terminator
					if err := f.Truncate(int64(totalBytesRead - 1)); err != nil {
						fmt.Println("Error truncating file:", err)
						return
					}
					io.WriteString(config.Writer, fmt.Sprintf("Finish receiving the file. Received %d total bytes.\n", totalBytesRead-1))
					return
				}
				if err != nil {
					io.WriteString(config.Writer, fmt.Sprintf("error receiving file: %v\n", err))
					return
				}
				logger.Debug("", "totalBytesRead", totalBytesRead)
			}
		}()
		return nil
	}
}
