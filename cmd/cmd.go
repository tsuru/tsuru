package cmd

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Manager struct {
	name     string
	commands map[string]interface{}
	Stdout   io.Writer
	Stderr   io.Writer
}

func (m *Manager) Register(command interface{}) {
	if m.commands == nil {
		m.commands = make(map[string]interface{})
	}
	name := command.(Infoer).Info().Name
	_, found := m.commands[name]
	if found {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	m.commands[name] = command
}

func (m *Manager) Run(args []string) {
	if len(args) == 0 {
		args = []string{"help"}
	}
	cmds := m.extractCommandFromArgs(args)
	if len(cmds) <= 0 {
		io.WriteString(m.Stderr, fmt.Sprintf("command %s does not exist\n", args[0]))
		return
	}
	args = args[len(cmds):]
	command := m.commands[cmds[0]]
	command = getSubcommand(command, cmds)
	if len(args) != command.(Infoer).Info().Args && cmds[0] != "help" {
		command = m.commands["help"]
		args = cmds
		cmds = []string{"help"}
	}
	err := command.(Command).Run(&Context{cmds, args, m.Stdout, m.Stderr}, NewClient(&http.Client{}))
	if err != nil {
		io.WriteString(m.Stderr, err.Error())
	}
}

func (m *Manager) extractCommandFromArgs(args []string) []string {
	cmds := []string{}
	if len(args) <= 0 {
		return cmds
	}
	if cmd, exists := m.commands[args[0]]; exists {
		cmds = append(cmds, args[0])
		if container, ok := cmd.(CommandContainer); ok && len(args) >= 2 {
			if _, exists = container.Subcommands()[args[1]]; exists {
				cmds = append(cmds, args[1])
			}
		}
	}
	return cmds
}

func getSubcommand(cmd interface{}, cmds []string) interface{} {
	if c, ok := cmd.(CommandContainer); ok && len(cmds) == 2 {
		if subcommand, exist := c.Subcommands()[cmds[1]]; exist {
			cmd = subcommand
		}
	}
	return cmd
}

func NewManager(name string, stdout, stderr io.Writer) Manager {
	m := Manager{name: name, Stdout: stdout, Stderr: stderr}
	m.Register(&Help{manager: &m})
	return m
}

type CommandContainer interface {
	Subcommands() map[string]interface{}
}

type Infoer interface {
	Info() *Info
}

type Command interface {
	Run(context *Context, client Doer) error
}

type Context struct {
	Cmds   []string
	Args   []string
	Stdout io.Writer
	Stderr io.Writer
}

type Info struct {
	Name  string
	Args  int
	Usage string
	Desc  string
}

type Help struct {
	manager *Manager
}

func (c *Help) Info() *Info {
	return &Info{
		Name:  "help",
		Usage: "command [args]",
	}
}

func (c *Help) Run(context *Context, client Doer) error {
	output := ""
	if len(context.Args) > 0 {
		cmd := c.manager.commands[context.Args[0]]
		cmd = getSubcommand(cmd, context.Args)
		info := cmd.(Infoer).Info()
		output = output + fmt.Sprintf("Usage: %s %s\n", c.manager.name, info.Usage)
		output = output + fmt.Sprintf("\n%s\n", info.Desc)
	} else {
		output = output + fmt.Sprintf("Usage: %s %s\n", c.manager.name, c.Info().Usage)
	}
	io.WriteString(context.Stdout, output)
	return nil
}

func ExtractProgramName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
