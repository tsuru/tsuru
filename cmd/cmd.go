// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	gerrors "errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/sajari/fuzzy"
	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/fs"
	"github.com/tsuru/tsuru/net"
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
	contexts      []*Context
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
	m.Register(&targetList{})
	m.Register(&targetAdd{})
	m.Register(&targetRemove{})
	m.Register(&targetSet{})
	m.Register(userInfo{})
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

type RemovedCommand struct {
	name string
	help string
}

func (c *RemovedCommand) Info() *Info {
	return &Info{
		Name:  c.name,
		Usage: c.name,
		Desc:  fmt.Sprintf("This command was removed. %s", c.help),
		fail:  true,
	}
}

func (c *RemovedCommand) Run(context *Context, client *Client) error {
	return ErrAbortCommand
}

func (m *Manager) RegisterRemoved(name string, help string) {
	if m.Commands == nil {
		m.Commands = make(map[string]Command)
	}
	_, found := m.Commands[name]
	if found {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	m.Commands[name] = &RemovedCommand{name: name, help: help}
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
	var (
		status         int
		verbosity      int
		displayHelp    bool
		displayVersion bool
	)
	if len(args) == 0 {
		args = append(args, "help")
	}
	flagset := gnuflag.NewFlagSet("tsuru flags", gnuflag.ContinueOnError)
	flagset.SetOutput(m.stderr)
	flagset.IntVar(&verbosity, "verbosity", 0, "Verbosity level: 1 => print HTTP requests; 2 => print HTTP requests/responses")
	flagset.IntVar(&verbosity, "v", 0, "Verbosity level: 1 => print HTTP requests; 2 => print HTTP requests/responses")
	flagset.BoolVar(&displayHelp, "help", false, "Display help and exit")
	flagset.BoolVar(&displayHelp, "h", false, "Display help and exit")
	flagset.BoolVar(&displayVersion, "version", false, "Print version and exit")
	parseErr := flagset.Parse(false, args)
	if parseErr != nil {
		fmt.Fprint(m.stderr, parseErr)
		m.finisher().Exit(2)
		return
	}
	args = flagset.Args()
	if displayHelp {
		args = append([]string{"help"}, args...)
	} else if displayVersion {
		args = []string{"version"}
	}
	name := args[0]
	command, ok := m.Commands[name]
	if !ok {
		if m.lookup != nil {
			context := m.newContext(args, m.stdout, m.stderr, m.stdin)
			err := m.lookup(context)
			if err != nil {
				msg := ""
				if os.IsNotExist(err) {
					msg = fmt.Sprintf("%s: %q is not a tsuru command. See %q.\n", os.Args[0], args[0], "tsuru help")
					var keys []string
					for key := range m.Commands {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					for _, key := range keys {
						levenshtein := fuzzy.Levenshtein(&key, &args[0])
						if levenshtein < 3 || strings.Contains(key, args[0]) {
							if !strings.Contains(msg, "Did you mean?") {
								msg += fmt.Sprintf("\nDid you mean?\n")
							}
							msg += fmt.Sprintf("\t%s\n", key)
						}
					}
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
	command, args, err := m.handleFlags(command, name, args)
	if err != nil {
		fmt.Fprint(m.stderr, err)
		m.finisher().Exit(1)
		return
	}
	if info.fail {
		command = m.Commands["help"]
		args = []string{name}
		status = 1
	}
	if length := len(args); (length < info.MinArgs || (info.MaxArgs > 0 && length > info.MaxArgs)) &&
		name != "help" {
		m.wrong = true
		m.original = info.Name
		command = m.Commands["help"]
		args = []string{name}
		status = 1
	}
	context := m.newContext(args, m.stdout, m.stderr, m.stdin)
	client := NewClient(net.Dial5FullUnlimitedClient, context, m)
	client.Verbosity = verbosity
	err = command.Run(context, client)
	if err == errUnauthorized && name != "login" {
		loginCmdName := "login"
		if cmd, ok := m.Commands[loginCmdName]; ok {
			fmt.Fprintln(m.stderr, "Error: you're not authenticated or your session has expired.")
			fmt.Fprintf(m.stderr, "Calling the %q command...\n", loginCmdName)
			loginContext := m.newContext(nil, m.stdout, m.stderr, m.stdin)
			if err = cmd.Run(loginContext, client); err == nil {
				fmt.Fprintln(m.stderr)
				err = command.Run(context, client)
			}
		}
	}
	if err != nil {
		errorMsg := err.Error()
		httpErr, ok := err.(*errors.HTTP)
		if ok && httpErr.Code == http.StatusUnauthorized && name != "login" {
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

func (m *Manager) newContext(args []string, stdout io.Writer, stderr io.Writer, stdin io.Reader) *Context {
	stdout = newPagerWriter(stdout)
	stdin = newSyncReader(stdin, stdout)
	ctx := &Context{args, stdout, stderr, stdin}
	m.contexts = append(m.contexts, ctx)
	return ctx
}

func (m *Manager) handleFlags(command Command, name string, args []string) (Command, []string, error) {
	var flagset *gnuflag.FlagSet
	if flagged, ok := command.(FlaggedCommand); ok {
		flagset = flagged.Flags()
	} else {
		flagset = gnuflag.NewFlagSet(name, gnuflag.ExitOnError)
	}
	var helpRequested bool
	flagset.SetOutput(m.stderr)
	if flagset.Lookup("help") == nil {
		flagset.BoolVar(&helpRequested, "help", false, "Display help and exit")
	}
	if flagset.Lookup("h") == nil {
		flagset.BoolVar(&helpRequested, "h", false, "Display help and exit")
	}
	err := flagset.Parse(true, args)
	if err != nil {
		return nil, nil, err
	}
	if helpRequested {
		command = m.Commands["help"]
		args = []string{name}
	} else {
		args = flagset.Args()
	}
	return command, args, nil
}

func (m *Manager) finisher() exiter {
	if pagerWriter, ok := m.stdout.(*pagerWriter); ok {
		pagerWriter.close()
	}
	for _, ctx := range m.contexts {
		if pagerWriter, ok := ctx.Stdout.(*pagerWriter); ok {
			pagerWriter.close()
		}
	}
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

func (c *Context) RawOutput() {
	if pager, ok := c.Stdout.(*pagerWriter); ok {
		c.Stdout = pager.baseWriter
	}
	if sync, ok := c.Stdin.(*syncReader); ok {
		c.Stdin = sync.baseReader
	}
}

type Info struct {
	Name    string
	MinArgs int
	MaxArgs int
	Usage   string
	Desc    string
	fail    bool
}

// Implementing the Commandable interface allows extending
// the tsurud command line interface
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
	return &Info{Name: "help", Usage: "command [args]"}
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
			flags := c.parseFlags(cmd)
			if flags != "" {
				output += fmt.Sprintf("\n%s", flags)
			}
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
		maxCmdSize := 20
		for _, command := range commands {
			if len(command) > maxCmdSize {
				maxCmdSize = len(command)
			}
		}
		for _, command := range commands {
			description := c.manager.Commands[command].Info().Desc
			description = strings.Split(description, "\n")[0]
			description = strings.Split(description, ".")[0]
			if len(description) > 2 {
				description = strings.ToUpper(description[:1]) + description[1:]
			}
			fmtStr := fmt.Sprintf("  %%-%ds %%s\n", maxCmdSize)
			output += fmt.Sprintf(fmtStr, command, description)
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

func (c *help) parseFlags(command Command) string {
	var output string
	if cmd, ok := command.(FlaggedCommand); ok {
		var buf bytes.Buffer
		flagset := cmd.Flags()
		flagset.SetOutput(&buf)
		flagset.PrintDefaults()
		if buf.String() != "" {
			output = fmt.Sprintf("Flags:\n\n%s", buf.String())
		}
	}
	return strings.Replace(output, "\n", "\n  ", -1)
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
