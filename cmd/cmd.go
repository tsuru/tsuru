package cmd

import (
	"fmt"
	"github.com/timeredbull/tsuru/fs"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
)

type Exiter interface {
	Exit(int)
}

type OsExiter struct{}

func (e OsExiter) Exit(code int) {
	os.Exit(code)
}

type Manager struct {
	Name     string
	Commands map[string]interface{}
	Stdout   io.Writer
	Stderr   io.Writer
	e        Exiter
}

func NewManager(name string, stdout, stderr io.Writer) Manager {
	m := Manager{Name: name, Stdout: stdout, Stderr: stderr}
	m.Register(&Help{manager: &m})
	return m
}

func BuildBaseManager(name string) Manager {
	m := NewManager(name, os.Stdout, os.Stderr)
	m.Register(&Login{})
	m.Register(&Logout{})
	m.Register(&User{})
	m.Register(&Team{})
	m.Register(&Target{})
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
	var status int
	if len(args) == 0 {
		args = []string{"help"}
	}
	cmds := m.extractCommandFromArgs(args)
	if len(cmds) <= 0 {
		io.WriteString(m.Stderr, fmt.Sprintf("command %s does not exist\n", args[0]))
		m.finisher().Exit(1)
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
		status = 1
	}
	if _, ok := command.(Command); !ok {
		io.WriteString(m.Stdout, fmt.Sprintf("subcommand %s does not exist\n\n", args[0]))
		command = m.Commands["help"]
		args = cmds
		cmds = []string{"help"}
		status = 1
	}
	err := command.(Command).Run(&Context{cmds, args, m.Stdout, m.Stderr}, NewClient(&http.Client{}))
	if err != nil {
		errorMsg := err.Error()
		if !strings.HasSuffix(errorMsg, "\n") {
			errorMsg += "\n"
		}
		io.WriteString(m.Stderr, errorMsg)
		status = 1
	}
	m.finisher().Exit(status)
}

func (mngr *Manager) extractCommandFromArgs(args []string) []string {
	cmds := []string{}
	if len(args) <= 0 {
		return cmds
	}
	if cmd, exists := mngr.Commands[args[0]]; exists {
		cmds = append(cmds, args[0])
		if container, ok := cmd.(CommandContainer); ok && len(args) >= 2 {
			cmds = append([]string{}, appendSubcmds(cmds, container, args, 1)...)
		}
	}
	return cmds
}

func appendSubcmds(cmds []string, container CommandContainer, args []string, i int) []string {
	var ok bool
	var inter interface{}
	if len(args) <= i {
		return cmds
	}
	if inter, ok = container.Subcommands()[args[i]]; !ok {
		return cmds
	}
	cmds = append(cmds, args[i])
	if container, ok = inter.(CommandContainer); !ok {
		return cmds
	}
	return appendSubcmds(cmds, container, args, i+1)
}

func (m *Manager) finisher() Exiter {
	if m.e == nil {
		m.e = OsExiter{}
	}
	return m.e
}

func getSubcommand(cmd interface{}, cmds []string) interface{} {
	i := 1
	return getSubcommandRecursive(cmd, cmds, i)
}

func getSubcommandRecursive(cmd interface{}, cmds []string, i int) interface{} {
	if c, ok := cmd.(CommandContainer); ok && len(cmds) >= 2 {
		if len(cmds) > i {
			if subcommand, exist := c.Subcommands()[cmds[i]]; exist {
				cmd = getSubcommandRecursive(subcommand, cmds, i+1)
			}
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
		output += fmt.Sprintf("Usage: %s %s\n", c.manager.Name, info.Usage)
		output += fmt.Sprintf("\n%s\n", info.Desc)
		if info.MinArgs > 0 {
			output += fmt.Sprintf("\nMinimum arguments: %d\n", info.MinArgs)
		}
	} else {
		output += fmt.Sprintf("Usage: %s %s\n\nAvailable commands:\n", c.manager.Name, c.Info().Usage)
		var commands []string
		for k, _ := range c.manager.Commands {
			commands = append(commands, k)
		}
		sort.Strings(commands)
		for _, command := range commands {
			output += fmt.Sprintf("  %s\n", command)
		}
		output += fmt.Sprintf("\nRun %s help <commandname> to get more information about a specific command.\n", c.manager.Name)
	}
	io.WriteString(context.Stdout, output)
	return nil
}

func ExtractProgramName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

var fsystem fs.Fs

func filesystem() fs.Fs {
	if fsystem == nil {
		fsystem = fs.OsFs{}
	}
	return fsystem
}
