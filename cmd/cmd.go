package cmd

import (
	"fmt"
	"io"
	"net/http"
)

type Manager struct {
	Name     string
	Commands map[string]interface{}
	Stdout   io.Writer
	Stderr   io.Writer
}

func NewManager(name string, stdout, stderr io.Writer) Manager {
	m := Manager{Name: name, Stdout: stdout, Stderr: stderr}
	m.Register(&Help{manager: &m})
	return m
}

func (m *Manager) Register(command interface{}) {
	if m.Commands == nil {
		m.Commands = make(map[string]interface{})
	}
	name := command.(Infoer).Info().Name
	_, found := m.Commands[name]
	if found {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	m.Commands[name] = command
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
	command := m.Commands[cmds[0]]
	command = getSubcommand(command, cmds)
	if len(args) < command.(Infoer).Info().MinArgs && cmds[0] != "help" {
		io.WriteString(m.Stdout, fmt.Sprintf("Not enough arguments to call %s.\n\n", command.(Infoer).Info().Name))
		command = m.Commands["help"]
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
	if cmd, exists := m.Commands[args[0]]; exists {
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
	Name    string
	MinArgs int
	Usage   string
	Desc    string
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
		cmd := c.manager.Commands[context.Args[0]]
		cmd = getSubcommand(cmd, context.Args)
		info := cmd.(Infoer).Info()
		output = output + fmt.Sprintf("Usage: %s %s\n", c.manager.Name, info.Usage)
		output = output + fmt.Sprintf("\n%s\n", info.Desc)
		if info.MinArgs > 0 {
			output = output + fmt.Sprintf("\nMinimum arguments: %d\n", info.MinArgs)
		}
	} else {
		output = output + fmt.Sprintf("Usage: %s %s\n", c.manager.Name, c.Info().Usage)
	}
	io.WriteString(context.Stdout, output)
	return nil
}
