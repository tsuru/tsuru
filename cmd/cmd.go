// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	gerrors "errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/fs"
	"launchpad.net/gnuflag"
)

var ErrAbortCommand = gerrors.New("")

type exiter interface {
	Exit(int)
}

type osExiter struct{}

func (e osExiter) Exit(code int) {
	os.Exit(code)
}

type Lookup func(context *Context) error

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
	lookup        Lookup
}

func NewManager(name, ver, verHeader string, stdout, stderr io.Writer, stdin io.Reader, lookup Lookup) *Manager {
	manager := &Manager{name: name, version: ver, versionHeader: verHeader, stdout: stdout, stderr: stderr, stdin: stdin, lookup: lookup}
	manager.Register(&help{manager})
	manager.Register(&version{manager})
	return manager
}

func BuildBaseManager(name, version, versionHeader string, lookup Lookup) *Manager {
	m := NewManager(name, version, versionHeader, os.Stdout, os.Stderr, os.Stdin, lookup)
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
	m.RegisterTopic("target", fmt.Sprintf(targetTopic, name))
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

func (m *Manager) RegisterDeprecated(command Command, oldName string) {
	if m.Commands == nil {
		m.Commands = make(map[string]Command)
	}
	name := command.Info().Name
	_, found := m.Commands[name]
	if found {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	m.Commands[name] = command
	m.Commands[oldName] = &DeprecatedCommand{Command: command, oldName: oldName}
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
		if m.lookup != nil {
			context := Context{args, m.stdout, m.stderr, m.stdin}
			err := m.lookup(&context)
			if err != nil {
				msg := ""
				if os.IsNotExist(err) {
					msg = fmt.Sprintf("Error: command %q does not exist\n", args[0])
				} else {
					msg = err.Error()
				}
				fmt.Fprint(m.stderr, msg)
				m.finisher().Exit(1)
			}
			return
		}
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
	if length := len(args); (length < info.MinArgs || (info.MaxArgs > 0 && length > info.MaxArgs)) &&
		name != "help" {
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
		errorMsg := err.Error()
		httpErr, ok := err.(*errors.HTTP)
		if ok && httpErr.Code == http.StatusUnauthorized {
			errorMsg = `You're not authenticated or your session has expired. Please use "login" command for authentication.`
		}
		if !strings.HasSuffix(errorMsg, "\n") {
			errorMsg += "\n"
		}
		if err != ErrAbortCommand {
			io.WriteString(m.stderr, "Error: "+errorMsg)
		}
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

type DeprecatedCommand struct {
	Command
	oldName string
}

func (c *DeprecatedCommand) Run(context *Context, client *Client) error {
	fmt.Fprintf(context.Stderr, "WARNING: %q has been deprecated, please use %q instead.\n\n", c.oldName, c.Command.Info().Name)
	return c.Command.Run(context, client)
}

func (c *DeprecatedCommand) Flags() *gnuflag.FlagSet {
	if cmd, ok := c.Command.(FlaggedCommand); ok {
		return cmd.Flags()
	}
	return gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
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
	MaxArgs int
	Usage   string
	Desc    string
}

// Implementing the Commandable interface allows extending
// the tsr command line interface
type Commandable interface {
	Commands() []Command
}

// Implementing the AdminCommandable interface allows extending
// the tsuru-admin command line interface
type AdminCommandable interface {
	AdminCommands() []Command
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
	const deprecatedMsg = "WARNING: %q is deprecated. Showing help for %q instead.\n\n"
	output := fmt.Sprintf("%s version %s.\n\n", c.manager.name, c.manager.version)
	if c.manager.wrong {
		output += fmt.Sprint("ERROR: wrong number of arguments.\n\n")
	}
	if len(context.Args) > 0 {
		if cmd, ok := c.manager.Commands[context.Args[0]]; ok {
			if deprecated, ok := cmd.(*DeprecatedCommand); ok {
				fmt.Fprintf(context.Stderr, deprecatedMsg, deprecated.oldName, cmd.Info().Name)
			}
			info := cmd.Info()
			output += fmt.Sprintf("Usage: %s %s\n", c.manager.name, info.Usage)
			output += fmt.Sprintf("\n%s\n", info.Desc)
			if info.MinArgs > 0 {
				output += fmt.Sprintf("\nMinimum # of arguments: %d", info.MinArgs)
			}
			if info.MaxArgs > 0 {
				output += fmt.Sprintf("\nMaximum # of arguments: %d", info.MaxArgs)
			}
			output += fmt.Sprint("\n")
		} else if topic, ok := c.manager.topics[context.Args[0]]; ok {
			output += topic
		} else {
			return fmt.Errorf("command %q does not exist.", context.Args[0])
		}
	} else {
		output += fmt.Sprintf("Usage: %s %s\n\nAvailable commands:\n", c.manager.name, c.Info().Usage)
		var commands []string
		for name, cmd := range c.manager.Commands {
			if _, ok := cmd.(*DeprecatedCommand); !ok {
				commands = append(commands, name)
			}
		}
		sort.Strings(commands)
		for _, command := range commands {
			output += fmt.Sprintf("  %s\n", command)
		}
		output += fmt.Sprintf("\nUse %s help <commandname> to get more information about a command.\n", c.manager.name)
		if len(c.manager.topics) > 0 {
			output += fmt.Sprintln("\nAvailable topics:")
			for topic := range c.manager.topics {
				output += fmt.Sprintf("  %s\n", topic)
			}
			output += fmt.Sprintf("\nUse %s help <topicname> to get more information about a topic.\n", c.manager.name)
		}
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
	if supported == "" || supported == current {
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
		if current > supported {
			return true
		}
	}
	if bigger {
		return false
	}
	return true
}
