package repl

// note: some of the code is based off csci1270 course project
import (
	"bufio"
	"fmt"
	"io"
	"iptcp-nora-yu/pkg/ipnode"
	"iptcp-nora-yu/pkg/proto"
	"net/netip"
	"os"
	"strings"
)

type replHandlerFunc func(string, *REPLConfig) error

type REPL struct {
	Commands map[string]replHandlerFunc
	Help     map[string]string
}

type REPLConfig struct {
	writer io.Writer
	node   *ipnode.Node
}

func NewRepl() *REPL {
	r := &REPL{make(map[string]replHandlerFunc), make(map[string]string)}
	r.addDefaultCommands()
	return r
}

// Add a command, along with its help string, to the set of commands
func (r *REPL) AddCommand(trigger string, handler replHandlerFunc, help string) {
	if trigger == "" || trigger[0] == '.' {
		return
	}
	r.Help[trigger] = help
	r.Commands[trigger] = handler
}

// Return all REPL usage information as a string
func (r *REPL) HelpString() string {
	var sb strings.Builder
	sb.WriteString("Commands\n")
	for k, v := range r.Help {
		sb.WriteString(fmt.Sprintf("\t%s: %s\n", k, v))
	}
	return sb.String()
}

func (r *REPL) Run(node *ipnode.Node) {
	reader := os.Stdin
	writer := os.Stdout
	scanner := bufio.NewScanner((reader))
	replConfig := &REPLConfig{writer: writer, node: node}

	// begin the repl
	io.WriteString(writer, ">") // the prompt
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if scanner.Err() != nil {
			break
		}
		command := strings.Split(input, " ")[0]
		handler, ok := r.Commands[command]

		if !ok {
			io.WriteString(writer, fmt.Sprintf("Invalid command: %s\n", command))
			io.WriteString(writer, r.HelpString())
		} else {
			err := handler(input, replConfig)
			if err != nil {
				io.WriteString(writer, fmt.Sprintf("Error: %v\n", err))
			}
		}

		io.WriteString(writer, ">")
	}
}

func (r *REPL) addDefaultCommands() {
	r.AddCommand("ln", lnHandler, "Prints neighbors of the current node. usage: ln")
	r.AddCommand("li", liHandler, "Prints all interfaces of the current node. usage: li")
	r.AddCommand("lr", lrHandler, "Prints the routing table of the current node. usage: lr")
	r.AddCommand("up", upHandler, "Sets the interface's state to up. usage: up <interface name>")
	r.AddCommand("down", downHandler, "Sets the interface's state to down. usage: down <interface name>")
	r.AddCommand("send", sendHandler, "Sends the messgae to the dest ip address. usage: send <dest IP> <msg>")
}

func lnHandler(input string, replConfig *REPLConfig) error {
	args := strings.Split(input, " ")
	if len(args) != 1 {
		return fmt.Errorf("usage: ln")
	}
	neighbors := replConfig.node.GetNeighborsString()
	writer := replConfig.writer

	_, err := io.WriteString(writer, "Iface\tVIP\t\tUDPAddr\n")
	// log.Printf("[lnHandler] writes %d bytes\n", bytes)
	if err != nil {
		return fmt.Errorf("lnHandler cannot write the header to stdout.\n")
	}

	for _, neighborInfo := range neighbors {
		_, err := io.WriteString(writer, neighborInfo)
		// log.Printf("[lnHandler] writes %d bytes\n", bytes)
		if err != nil {
			return fmt.Errorf("lnHandler cannot write neighors to stdout.\n")
		}
	}
	return nil
}

func liHandler(input string, replConfig *REPLConfig) error {
	args := strings.Split(input, " ")
	if len(args) != 1 {
		return fmt.Errorf("usage: li")
	}
	interfaces := replConfig.node.GetInterfacesString()
	writer := replConfig.writer

	_, err := io.WriteString(writer, "Name\tAddr/Prefix\tState\n")
	// log.Printf("[liHandler] writes %d bytes\n", bytes)
	if err != nil {
		return fmt.Errorf("liHandler cannot write the header to stdout.\n")
	}

	for _, interfaceInfo := range interfaces {
		_, err := io.WriteString(writer, interfaceInfo)
		// log.Printf("[liHandler] writes %d bytes", bytes)
		if err != nil {
			return fmt.Errorf("liHandler cannot write interfaces to stdout.\n")
		}
	}
	return nil
}

func lrHandler(input string, replConfig *REPLConfig) error {
	args := strings.Split(input, " ")
	if len(args) != 1 {
		return fmt.Errorf("usage: lr")
	}
	routingTable := replConfig.node.GetRoutingTableString()
	writer := replConfig.writer

	_, err := io.WriteString(writer, "T\tPrefix\t\tNext hop\tCost\n")
	// log.Printf("[lrHandler] writes %d bytes\n", bytes)
	if err != nil {
		return fmt.Errorf("lrHandler cannot write the header to stdout.\n")
	}

	for _, rtInfo := range routingTable {
		_, err := io.WriteString(writer, rtInfo)
		// log.Printf("[lrHandler] writes %d bytes", bytes)
		if err != nil {
			return fmt.Errorf("lrHandler cannot write routing table to stdout.\n")
		}
	}
	return nil
}

func upHandler(input string, replConfig *REPLConfig) error {
	args := strings.Split(input, " ")
	if len(args) != 2 {
		return fmt.Errorf("usage: up <interface name>")
	}
	interfaceName := args[1]

	err := replConfig.node.SetInterfaceIsDown(interfaceName, false)
	if err != nil {
		return err
	}
	return nil
}

func downHandler(input string, replConfig *REPLConfig) error {
	args := strings.Split(input, " ")
	if len(args) != 2 {
		return fmt.Errorf("usage: down <interface name>")
	}
	interfaceName := args[1]

	err := replConfig.node.SetInterfaceIsDown(interfaceName, true)
	if err != nil {
		return err
	}
	return nil
}

func sendHandler(input string, replConfig *REPLConfig) error {
	args := strings.Split(input, " ")
	if len(args) < 3 {
		return fmt.Errorf("usage: send <dest IP> <msg>")
	}
	msg := strings.Join(args[2:], " ")
	err := replConfig.node.Send(netip.MustParseAddr(args[1]), msg, proto.TestProtoNum)
	if err != nil {
		return err
	}
	return nil
}
