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

type exiter interface {
	Exit(int)
}

type osExiter struct{}

func (e osExiter) Exit(code int) {
	os.Exit(code)
}

type Manager struct {
	Name     string
	Commands map[string]interface{}
	Stdout   io.Writer
	Stderr   io.Writer
	version  string
	e        exiter
	original string
	wrong    bool
}

func NewManager(name, ver string, stdout, stderr io.Writer) *Manager {
	manager := &Manager{Name: name, version: ver, Stdout: stdout, Stderr: stderr}
	manager.Register(&help{manager})
	manager.Register(&version{manager})
	return manager
}

func BuildBaseManager(name, version string) *Manager {
	m := NewManager(name, version, os.Stdout, os.Stderr)
	m.Register(&login{})
	m.Register(&logout{})
	m.Register(&userCreate{})
	m.Register(&teamCreate{})
	m.Register(&teamList{})
	m.Register(&teamUserAdd{})
	m.Register(&teamUserRemove{})
	m.Register(&target{})
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
		args = append(args, "help")
	}
	name := args[0]
	command, ok := m.Commands[name]
	if !ok {
		io.WriteString(m.Stderr, fmt.Sprintf("command %s does not exist\n", args[0]))
		m.finisher().Exit(1)
		return
	}
	args = args[1:]
	if len(args) < command.(Infoer).Info().MinArgs && name != "help" {
		m.wrong = true
		m.original = command.(Infoer).Info().Name
		command = m.Commands["help"]
		args = []string{name}
		status = 1
	}
	err := command.(Command).Run(&Context{nil, args, m.Stdout, m.Stderr}, NewClient(&http.Client{}))
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

func (m *Manager) finisher() exiter {
	if m.e == nil {
		m.e = osExiter{}
	}
	return m.e
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

type help struct {
	manager *Manager
}

func (c *help) Info() *Info {
	return &Info{
		Name:  "help",
		Usage: "command [args]",
	}
}

func (c *help) Run(context *Context, client Doer) error {
	output := fmt.Sprintf("%s version %s.\n\n", c.manager.Name, c.manager.version)
	if c.manager.wrong {
		output += fmt.Sprintf("ERROR: not enough arguments to call %s.\n\n", c.manager.original)
	}
	if len(context.Args) > 0 {
		cmd, ok := c.manager.Commands[context.Args[0]]
		if !ok {
			return fmt.Errorf("Command %s does not exist.", context.Args[0])
		}
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

type version struct {
	manager *Manager
}

func (c *version) Info() *Info {
	return &Info{
		Name:    "version",
		MinArgs: 0,
		Usage:   "version",
		Desc:    "display the current version",
	}
}

func (c *version) Run(context *Context, client Doer) error {
	fmt.Fprintf(context.Stdout, "%s version %s.\n", c.manager.Name, c.manager.version)
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
