package repl

// note: some of the code is based off csci1270 course project
import (
	"bufio"
	"fmt"
	"io"
	"iptcp-nora-yu/pkg/ipnode"
	"log"
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
	return &REPL{make(map[string]replHandlerFunc), make(map[string]string)}
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

	// add default commands here
	r.AddCommand("ln", func(s string, r *REPLConfig) error {
		return lnHandler(s, replConfig)
	}, "Prints neighbors of the current node. usage: ln")
	r.AddCommand("li", func(s string, r *REPLConfig) error {
		return liHandler(s, replConfig)
	}, "Prints all interfaces of the current node. usage: li")
	r.AddCommand("lr", func(s string, r *REPLConfig) error {
		return lrHandler(s, replConfig)
	}, "Prints the routing table of the current node. usage: lr")
	r.AddCommand("up", func(s string, r *REPLConfig) error {
		return upHandler(s, replConfig)
	}, "Sets the interface's state to up. usage: up")
	r.AddCommand("down", func(s string, r *REPLConfig) error {
		return downHandler(s, replConfig)
	}, "Sets the interface's state to down. usage: down")
	r.AddCommand("send", func(s string, r *REPLConfig) error {
		return sendHandler(s, replConfig)
	}, "Sends the messgae to the dest ip address. usage: send")

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

// ---------- some shared handler (might wanna move these elsewhere)
func lnHandler(input string, replConfig *REPLConfig) error {
	args := strings.Split(input, " ")
	if len(args) != 1 {
		return fmt.Errorf("usage: ln")
	}
	neighbors := replConfig.node.GetNeighborsString()
	rowSize := len(neighbors)
	writer := replConfig.writer

	bytes, err := io.WriteString(writer, "Iface          VIP          UDPAddr\n")
	log.Printf("[lnHandler] writes %d bytes", bytes)
	if err != nil {
		return fmt.Errorf("lnHandler cannot write the header to stdout.\n")
	}

	for i := 0; i < rowSize; i++ {
		row := neighbors[i]
		bytes, err := io.WriteString(writer, fmt.Sprintf("%s     %s   %s\n", row[0], row[1], row[2]))
		log.Printf("[lnHandler] writes %d bytes", bytes)
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
	bytes, err := io.WriteString(writer, "Name  Addr/Prefix State\n")
	log.Printf("[liHandler] writes %d bytes", bytes)
	if err != nil {
		return fmt.Errorf("liHandler cannot write the header to stdout.\n")
	}

	for i := 0; i < len(interfaces); i++ {
		row := interfaces[i]
		bytes, err := io.WriteString(writer, fmt.Sprintf("%s     %s   %s\n", row[0], row[1], row[2]))
		log.Printf("[liHandler] writes %d bytes", bytes)
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
	bytes, err := io.WriteString(writer, "T       Prefix   Next hop   Cost\n")
	log.Printf("[lrHandler] writes %d bytes", bytes)
	if err != nil {
		return fmt.Errorf("lrHandler cannot write the header to stdout.\n")
	}

	for i := 0; i < len(routingTable); i++ {
		row := routingTable[i]
		bytes, err := io.WriteString(writer, fmt.Sprintf("%s  %s   %s     %s\n", row[0], row[1], row[2], row[3]))
		log.Printf("[lrHandler] writes %d bytes", bytes)
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
	if len(args) != 3 {
		return fmt.Errorf("usage: send <dest IP> <msg>")
	}
	err := replConfig.node.Send(netip.MustParseAddr(args[1]), args[2], 0)
	if err != nil {
		return err
	}
	return nil
}
