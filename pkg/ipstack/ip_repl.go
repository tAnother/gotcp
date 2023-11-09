package ipstack

import (
	"fmt"
	"io"
	"iptcp-nora-yu/pkg/proto"
	"iptcp-nora-yu/pkg/repl"
	"net/netip"
	"strings"
)

func IpRepl(s *IPGlobalInfo) *repl.REPL {
	r := repl.NewRepl()
	r.AddCommand("ln", s.lnHandler(), "Prints neighbors of the current s. usage: ln")
	r.AddCommand("li", s.liHandler(), "Prints all interfaces of the current s. usage: li")
	r.AddCommand("lr", s.lrHandler(), "Prints the routing table of the current s. usage: lr")
	r.AddCommand("up", s.upHandler(), "Sets the interface's state to up. usage: up <interface name>")
	r.AddCommand("down", s.downHandler(), "Sets the interface's state to down. usage: down <interface name>")
	r.AddCommand("send", s.sendHandler(), "Sends the messgae to the dest ip address. usage: send <dest IP> <msg>")
	return r
}

func (s *IPGlobalInfo) lnHandler() func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 1 {
			return fmt.Errorf("usage: ln")
		}
		neighbors := s.GetNeighborsString()

		_, err := io.WriteString(config.Writer, "Iface\tVIP\t\tUDPAddr\n")
		if err != nil {
			return fmt.Errorf("lnHandler cannot write the header to stdout")
		}

		for _, neighborInfo := range neighbors {
			_, err := io.WriteString(config.Writer, neighborInfo)
			if err != nil {
				return fmt.Errorf("lnHandler cannot write neighors to stdout")
			}
		}
		return nil
	}
}

func (s *IPGlobalInfo) liHandler() func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 1 {
			return fmt.Errorf("usage: li")
		}
		interfaces := s.GetInterfacesString()

		_, err := io.WriteString(config.Writer, "Name\tAddr/Prefix\tState\n")
		if err != nil {
			return fmt.Errorf("liHandler cannot write the header to stdout")
		}

		for _, interfaceInfo := range interfaces {
			_, err := io.WriteString(config.Writer, interfaceInfo)
			if err != nil {
				return fmt.Errorf("liHandler cannot write interfaces to stdout")
			}
		}
		return nil
	}
}

func (s *IPGlobalInfo) lrHandler() func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 1 {
			return fmt.Errorf("usage: lr")
		}
		routingTable := s.GetRoutingTableString()

		_, err := io.WriteString(config.Writer, "T\tPrefix\t\tNext hop\tCost\n")
		if err != nil {
			return fmt.Errorf("lrHandler cannot write the header to stdout")
		}

		for _, rtInfo := range routingTable {
			_, err := io.WriteString(config.Writer, rtInfo)
			// log.Printf("[lrHandler] writes %d bytes", bytes)
			if err != nil {
				return fmt.Errorf("lrHandler cannot write routing table to stdout")
			}
		}
		return nil
	}
}

func (s *IPGlobalInfo) upHandler() func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 2 {
			return fmt.Errorf("usage: up <interface name>")
		}
		interfaceName := args[1]

		err := s.SetInterfaceIsDown(interfaceName, false)
		if err != nil {
			return err
		}
		return nil
	}
}

func (s *IPGlobalInfo) downHandler() func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) != 2 {
			return fmt.Errorf("usage: down <interface name>")
		}
		interfaceName := args[1]

		err := s.SetInterfaceIsDown(interfaceName, true)
		if err != nil {
			return err
		}
		return nil
	}
}

func (s *IPGlobalInfo) sendHandler() func(string, *repl.REPLConfig) error {
	return func(input string, config *repl.REPLConfig) error {
		args := strings.Split(input, " ")
		if len(args) < 3 {
			return fmt.Errorf("usage: send <dest IP> <msg>")
		}
		msg := strings.Join(args[2:], " ")
		err := s.Send(netip.MustParseAddr(args[1]), []byte(msg), proto.ProtoNumTest)
		if err != nil {
			return err
		}
		return nil
	}
}
