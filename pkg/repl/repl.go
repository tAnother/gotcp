package repl

// note: based off of csci1270-fall23
import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type REPL struct {
	Commands map[string]func(string, *REPLConfig) error
	Help     map[string]string
}

type REPLConfig struct {
	Writer io.Writer
}

func NewRepl() *REPL {
	r := &REPL{make(map[string]func(string, *REPLConfig) error), make(map[string]string)}
	return r
}

// Add a command, along with its help string, to the set of commands
func (r *REPL) AddCommand(trigger string, handler func(string, *REPLConfig) error, help string) {
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

func (r *REPL) Run() {
	reader := os.Stdin
	writer := os.Stdout
	scanner := bufio.NewScanner((reader))
	replConfig := &REPLConfig{Writer: writer}

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
