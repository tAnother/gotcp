package repl

// note: some of the code is based off csci1270 course project
import (
	"bufio"
	"fmt"
	"io"
	"iptcp-nora-yu/pkg/ipnode"
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
func lnHandler(input string, replConfig *REPLConfig) {

}

func liHandler(input string, replConfig *REPLConfig) {

}

func upHandler(input string, replConfig *REPLConfig) {

}

func downHandler(input string, replConfig *REPLConfig) {

}

func sendHandler(input string, replConfig *REPLConfig) {

}
