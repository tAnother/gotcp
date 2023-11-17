package repl

// note: based off of csci1270-fall23
import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chzyer/readline"
)

type REPL struct {
	rl       *readline.Instance
	Commands map[string]func(string, *REPLConfig) error
	Help     map[string]string
}

type REPLConfig struct {
	Writer io.Writer
}

func NewRepl() *REPL {
	l, err := readline.NewEx(&readline.Config{
		Prompt:            "> ",
		HistoryFile:       "/tmp/repl.tmp",
		InterruptPrompt:   "^C",
		HistorySearchFold: true,
	})
	if err != nil {
		panic(err)
	}
	r := &REPL{
		rl:       l,
		Commands: make(map[string]func(string, *REPLConfig) error),
		Help:     make(map[string]string),
	}
	return r
}

// Combines a slice of REPLs.
func CombineRepls(repls []*REPL) (*REPL, error) {
	combined := NewRepl()
	if repls == nil || len(repls) < 1 {
		return combined, nil
	}
	for _, repl := range repls {
		for k, v := range repl.Help {
			if _, ok := combined.Help[k]; ok {
				return nil, errors.New("overlaps in repls")
			}
			combined.Help[k] = v
			combined.Commands[k] = repl.Commands[k]
		}
	}
	return combined, nil
}

// Add a command, along with its help string, to the set of commands
func (r *REPL) AddCommand(trigger string, handler func(string, *REPLConfig) error, help string) {
	if trigger == "" {
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
	writer := os.Stdout
	replConfig := &REPLConfig{Writer: writer}

	for {
		input, err := r.rl.Readline()
		if err == readline.ErrInterrupt {
			return
		} else if err == io.EOF {
			return
		}

		input = strings.TrimSpace(input)
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
	}
}
