package ipnode

import (
	"fmt"
	"io"
	"iptcp-nora-yu/pkg/proto"
	"iptcp-nora-yu/pkg/repl"
	"net/netip"
	"strings"
)

func NodeRepl(node *Node) *repl.REPL {
	r := repl.NewRepl()
	r.AddCommand("ln", lnHandler(node), "Prints neighbors of the current node. usage: ln")
	r.AddCommand("li", liHandler(node), "Prints all interfaces of the current node. usage: li")
	r.AddCommand("lr", lrHandler(node), "Prints the routing table of the current node. usage: lr")
	r.AddCommand("up", upHandler(node), "Sets the interface's state to up. usage: up <interface name>")
	r.AddCommand("down", downHandler(node), "Sets the interface's state to down. usage: down <interface name>")
	r.AddCommand("send", sendHandler(node), "Sends the messgae to the dest ip address. usage: send <dest IP> <msg>")
	return r
}

func lnHandler(node *Node) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 1 {
			return fmt.Errorf("usage: ln")
		}
		neighbors := node.GetNeighborsString()

		_, err := io.WriteString(config.Writer, "Iface\tVIP\t\tUDPAddr\n")
		if err != nil {
			return fmt.Errorf("lnHandler cannot write the header to stdout.\n")
		}

		for _, neighborInfo := range neighbors {
			_, err := io.WriteString(config.Writer, neighborInfo)
			if err != nil {
				return fmt.Errorf("lnHandler cannot write neighors to stdout.\n")
			}
		}
		return nil
	}
}

func liHandler(node *Node) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 1 {
			return fmt.Errorf("usage: li")
		}
		interfaces := node.GetInterfacesString()

		_, err := io.WriteString(config.Writer, "Name\tAddr/Prefix\tState\n")
		if err != nil {
			return fmt.Errorf("liHandler cannot write the header to stdout.\n")
		}

		for _, interfaceInfo := range interfaces {
			_, err := io.WriteString(config.Writer, interfaceInfo)
			if err != nil {
				return fmt.Errorf("liHandler cannot write interfaces to stdout.\n")
			}
		}
		return nil
	}
}

func lrHandler(node *Node) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 1 {
			return fmt.Errorf("usage: lr")
		}
		routingTable := node.GetRoutingTableString()

		_, err := io.WriteString(config.Writer, "T\tPrefix\t\tNext hop\tCost\n")
		if err != nil {
			return fmt.Errorf("lrHandler cannot write the header to stdout.\n")
		}

		for _, rtInfo := range routingTable {
			_, err := io.WriteString(config.Writer, rtInfo)
			// log.Printf("[lrHandler] writes %d bytes", bytes)
			if err != nil {
				return fmt.Errorf("lrHandler cannot write routing table to stdout.\n")
			}
		}
		return nil
	}
}

func upHandler(node *Node) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 2 {
			return fmt.Errorf("usage: up <interface name>")
		}
		interfaceName := args[1]

		err := node.SetInterfaceIsDown(interfaceName, false)
		if err != nil {
			return err
		}
		return nil
	}
}

func downHandler(node *Node) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 2 {
			return fmt.Errorf("usage: down <interface name>")
		}
		interfaceName := args[1]

		err := node.SetInterfaceIsDown(interfaceName, true)
		if err != nil {
			return err
		}
		return nil
	}
}

func sendHandler(node *Node) func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) < 3 {
			return fmt.Errorf("usage: send <dest IP> <msg>")
		}
		msg := strings.Join(args[2:], " ")
		err := node.Send(netip.MustParseAddr(args[1]), []byte(msg), proto.ProtoNumTest)
		if err != nil {
			return err
		}
		return nil
	}
}
