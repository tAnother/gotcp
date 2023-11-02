package tcpstack

import (
	"fmt"
	"io"
	"iptcp-nora-yu/pkg/ipnode"
	"iptcp-nora-yu/pkg/repl"
	"net/netip"
	"strconv"
	"strings"
)

func TcpRepl(node *ipnode.Node) *repl.REPL {
	r := repl.NewRepl()
	r.AddCommand("a", acceptHandler(node), "Listens and accpets a port. usage: a <port>")
	r.AddCommand("ls", lsHandler(node), "Lists all sockets. usage: ls")
	r.AddCommand("c", connectHandler(node), "Creates a new socket that connects to the specified virtual IP address and port. usage: c <vip> <port>")

	return r
}

func acceptHandler(node *ipnode.Node) func(string, *repl.REPLConfig) error {
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

		listener, err := VListen(uint16(port))
		if err != nil {
			return err
		}

		go func() error {
			for {
				_, err := listener.VAccept()
				if err != nil {
					return err
				}
			}
		}()
		return nil
	}
}

func connectHandler(node *ipnode.Node) func(string, *repl.REPLConfig) error {
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

		go func() error {
			_, err := VConnect(netip.MustParseAddr(args[1]), uint16(port))
			if err != nil {
				return err
			}
			return nil
		}()
		return nil
	}
}

func lsHandler(node *ipnode.Node) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 1 {
			return fmt.Errorf("usage: ls")
		}

		sockets := GetSocketTableString()

		_, err := io.WriteString(config.Writer, "SIDt\tLAddr\tLPort\tRAddr\tRPort\tStatus\n")
		if err != nil {
			return fmt.Errorf("lsHandler cannot write the header to stdout")
		}

		for _, socketsInfo := range sockets {
			_, err := io.WriteString(config.Writer, socketsInfo)
			if err != nil {
				return fmt.Errorf("lsHandler cannot write sockets to stdout")
			}
		}
		return nil
	}
}
