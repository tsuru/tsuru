package app

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/unit"
	"regexp"
	"strings"
	"sync"
)

const (
	ChanSize    = 10
	runAttempts = 5
)

var envLocker sync.Mutex

type Message struct {
	app     *App
	env     map[string]string
	kind    string
	success chan bool
}

var env chan Message = make(chan Message, ChanSize)

type Cmd struct {
	cmd    string
	result chan CmdResult
	u      unit.Unit
}

type CmdResult struct {
	err    error
	output []byte
}

var cmds chan Cmd = make(chan Cmd)

func init() {
	go collectEnvVars()
	go runCommands()
}

func runCommands() {
	for cmd := range cmds {
		out, err := cmd.u.Command(cmd.cmd)
		if cmd.result != nil {
			r := CmdResult{output: out, err: err}
			cmd.result <- r
		}
	}
}

func runCmd(cmd string, msg Message) {
	c := Cmd{
		u:      msg.app.unit(),
		cmd:    cmd,
		result: make(chan CmdResult),
	}
	cmds <- c
	var r CmdResult
	r = <-c.result
	for i := 0; r.err != nil && i < runAttempts; i++ {
		cmds <- c
		r = <-c.result
	}
	if msg.success != nil {
		msg.success <- r.err == nil
	}
}

func setEnvVar(msg Message) {
	envLocker.Lock()
	defer envLocker.Unlock()
	cmd := fmt.Sprintf("cat >> $HOME/%s.env <<END\n", msg.app.Name)
	for k, v := range msg.env {
		cmd += fmt.Sprintf(`export %s="%s"`+"\n", k, v)
	}
	cmd += "END\n"
	runCmd(cmd, msg)
}

// excludeLines filters the input excluding all lines that matches the given
// pattern.
//
// The pattern must be a valid regular expression. If the given regular
// expression is invalid, excludeLines will panic.
func excludeLines(input []byte, pattern string) []byte {
	regex := regexp.MustCompile(pattern)
	return filterOutput(input, func(line []byte) bool {
		return !regex.Match(line)
	})
}

func unsetEnvVar(msg Message) {
	envLocker.Lock()
	defer envLocker.Unlock()
	var variables []string
	for k, _ := range msg.env {
		variables = append(variables, k)
	}
	c := Cmd{
		cmd:    fmt.Sprintf("cat $HOME/%s.env", msg.app.Name),
		result: make(chan CmdResult),
		u:      msg.app.unit(),
	}
	cmds <- c
	r := <-c.result
	output := excludeLines(r.output, fmt.Sprintf(`^export (%s)=`, strings.Join(variables, "|")))
	cmd := fmt.Sprintf("cat > $HOME/%s.env <<END\n", msg.app.Name)
	cmd += string(output)
	cmd += "\nEND"
	runCmd(cmd, msg)
}

func collectEnvVars() {
	for e := range env {
		switch e.kind {
		case "set":
			setEnvVar(e)
		case "unset":
			unsetEnvVar(e)
		}
	}
}
