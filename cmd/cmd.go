// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"github.com/globocom/tsuru/fs"
	"io"
	"launchpad.net/gnuflag"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
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
	Commands      map[string]Command
	topics        map[string]string
	name          string
	stdout        io.Writer
	stderr        io.Writer
	stdin         io.Reader
	version       string
	versionHeader string
	e             exiter
	original      string
	wrong         bool
}

func NewManager(name, ver, verHeader string, stdout, stderr io.Writer, stdin io.Reader) *Manager {
	manager := &Manager{name: name, version: ver, versionHeader: verHeader, stdout: stdout, stderr: stderr, stdin: stdin}
	manager.Register(&help{manager})
	manager.Register(&version{manager})
	return manager
}

func BuildBaseManager(name, version, versionHeader string) *Manager {
	m := NewManager(name, version, versionHeader, os.Stdout, os.Stderr, os.Stdin)
	m.Register(&login{})
	m.Register(&logout{})
	m.Register(&userCreate{})
	m.Register(&resetPassword{})
	m.Register(&userRemove{})
	m.Register(&teamCreate{})
	m.Register(&teamRemove{})
	m.Register(&teamList{})
	m.Register(&teamUserAdd{})
	m.Register(&teamUserRemove{})
	m.Register(teamUserList{})
	m.Register(&changePassword{})
	m.Register(&targetList{})
	m.Register(&targetAdd{})
	m.Register(&targetRemove{})
	m.Register(&targetSet{})
	return m
}

func (m *Manager) Register(command Command) {
	if m.Commands == nil {
		m.Commands = make(map[string]Command)
	}
	name := command.Info().Name
	_, found := m.Commands[name]
	if found {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	m.Commands[name] = command
}

func (m *Manager) RegisterTopic(name, content string) {
	if m.topics == nil {
		m.topics = make(map[string]string)
	}
	_, found := m.topics[name]
	if found {
		panic(fmt.Sprintf("topic already registered: %s", name))
	}
	m.topics[name] = content
}

func (m *Manager) Run(args []string) {
	var status int
	if len(args) == 0 {
		args = append(args, "help")
	}
	name := args[0]
	command, ok := m.Commands[name]
	if !ok {
		fmt.Fprintf(m.stderr, "Error: command %q does not exist\n", args[0])
		m.finisher().Exit(1)
		return
	}
	args = args[1:]
	info := command.Info()
	if flagged, ok := command.(FlaggedCommand); ok {
		flagset := flagged.Flags()
		err := flagset.Parse(true, args)
		if err != nil {
			fmt.Fprint(m.stderr, err)
			m.finisher().Exit(1)
			return
		}
		args = flagset.Args()
	}
	if len(args) < info.MinArgs && name != "help" {
		m.wrong = true
		m.original = info.Name
		command = m.Commands["help"]
		args = []string{name}
		status = 1
	}
	context := Context{args, m.stdout, m.stderr, m.stdin}
	client := NewClient(&http.Client{}, &context, m)
	err := command.Run(&context, client)
	if err != nil {
		re := regexp.MustCompile(`^((Invalid token)|(You must provide the Authorization header))`)
		errorMsg := err.Error()
		if re.MatchString(errorMsg) {
			errorMsg = `You're not authenticated or your session has expired. Please use "login" command for authentication.`
		}
		if !strings.HasSuffix(errorMsg, "\n") {
			errorMsg += "\n"
		}
		io.WriteString(m.stderr, "Error: "+errorMsg)
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

type Command interface {
	Info() *Info
	Run(context *Context, client *Client) error
}

type FlaggedCommand interface {
	Command
	Flags() *gnuflag.FlagSet
}

type Context struct {
	Args   []string
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
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

func (c *help) Run(context *Context, client *Client) error {
	output := fmt.Sprintf("%s version %s.\n\n", c.manager.name, c.manager.version)
	if c.manager.wrong {
		output += fmt.Sprintf("ERROR: not enough arguments to call %s.\n\n", c.manager.original)
	}
	if len(context.Args) > 0 {
		if cmd, ok := c.manager.Commands[context.Args[0]]; ok {
			info := cmd.Info()
			output += fmt.Sprintf("Usage: %s %s\n", c.manager.name, info.Usage)
			output += fmt.Sprintf("\n%s\n", info.Desc)
			if info.MinArgs > 0 {
				output += fmt.Sprintf("\nMinimum arguments: %d\n", info.MinArgs)
			}
		} else if topic, ok := c.manager.topics[context.Args[0]]; ok {
			output += topic
		} else {
			return fmt.Errorf("command %q does not exist.", context.Args[0])
		}
	} else {
		output += fmt.Sprintf("Usage: %s %s\n\nAvailable commands:\n", c.manager.name, c.Info().Usage)
		var commands []string
		for k := range c.manager.Commands {
			commands = append(commands, k)
		}
		sort.Strings(commands)
		for _, command := range commands {
			output += fmt.Sprintf("  %s\n", command)
		}
		output += fmt.Sprintf("\nRun %s help <commandname> to get more information about a specific command.\n", c.manager.name)
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

func (c *version) Run(context *Context, client *Client) error {
	fmt.Fprintf(context.Stdout, "%s version %s.\n", c.manager.name, c.manager.version)
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

// validateVersion checks whether current version is greater or equal to
// supported version.
func validateVersion(supported, current string) bool {
	var (
		bigger bool
		limit  int
	)
	if supported == "" {
		return true
	}
	partsSupported := strings.Split(supported, ".")
	partsCurrent := strings.Split(current, ".")
	if len(partsSupported) > len(partsCurrent) {
		limit = len(partsCurrent)
		bigger = true
	} else {
		limit = len(partsSupported)
	}
	for i := 0; i < limit; i++ {
		current, err := strconv.Atoi(partsCurrent[i])
		if err != nil {
			return false
		}
		supported, err := strconv.Atoi(partsSupported[i])
		if err != nil {
			return false
		}
		if current < supported {
			return false
		}
	}
	if bigger {
		return false
	}
	return true
}
