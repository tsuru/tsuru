package app

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/unit"
)

const (
	ChanSize    = 10
	runAttempts = 5
)

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

func collectEnvVars() {
	for e := range env {
		cmd := fmt.Sprintf("cat >> $HOME/%s.env <<END\n", e.app.Name)
		for k, v := range e.env {
			cmd += fmt.Sprintf(`export %s="%s"`+"\n", k, v)
		}
		cmd += "END\n"
		c := Cmd{
			u:      e.app.unit(),
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
		if e.success != nil {
			e.success <- r.err == nil
		}
	}
}
