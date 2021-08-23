// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"

	goVersion "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/sajari/fuzzy"
	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/fs"
	"github.com/tsuru/tsuru/net"
)

var (
	ErrAbortCommand = errors.New("")

	// ErrLookup is the error that should be returned by lookup functions when it
	// cannot find a matching command for the given parameters.
	ErrLookup = errors.New("lookup error - command not found")
)

const (
	loginCmdName = "login"
)

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
	topicCommands map[string][]Command
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
	manager := &Manager{
		name:          name,
		version:       ver,
		versionHeader: verHeader,
		stdout:        stdout,
		stderr:        stderr,
		stdin:         stdin,
		lookup:        lookup,
		topics:        map[string]string{},
		topicCommands: map[string][]Command{},
	}
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
	m.RegisterTopic("target", targetTopic)
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

	parts := strings.Split(name, "-")

	for i := 1; i < len(parts); i++ {
		topic := strings.Join(parts[0:i], " ")
		if _, ok := m.topics[topic]; !ok {
			m.topics[topic] = ""
		}

		m.topicCommands[topic] = append(m.topicCommands[topic], command)
	}
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
	Name string
	Help string
}

func (c *RemovedCommand) Info() *Info {
	return &Info{
		Name:  c.Name,
		Usage: c.Name,
		Desc:  fmt.Sprintf("This command was removed. %s", c.Help),
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
	m.Commands[name] = &RemovedCommand{Name: name, Help: help}
}

func (m *Manager) RegisterTopic(name, content string) {
	if m.topics == nil {
		m.topics = make(map[string]string)
	}
	value := m.topics[name]
	if value != "" {
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
		target         string
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
	flagset.StringVar(&target, "t", "", "Define target for running command")
	flagset.StringVar(&target, "target", "", "Define target for running command")
	parseErr := flagset.Parse(false, args)
	if parseErr != nil {
		fmt.Fprint(m.stderr, parseErr)
		m.finisher().Exit(2)
		return
	}
	args = flagset.Args()
	args = m.normalizeCommandArgs(args)
	if displayHelp {
		args = append([]string{"help"}, args...)
	} else if displayVersion {
		args = []string{"version"}
	}
	if len(target) > 0 {
		os.Setenv("TSURU_TARGET", target)
	}
	if m.lookup != nil {
		context := m.newContext(args, m.stdout, m.stderr, m.stdin)
		err := m.lookup(context)
		if err != nil && err != ErrLookup {
			fmt.Fprint(m.stderr, err)
			m.finisher().Exit(1)
			return
		} else if err == nil {
			return
		}
	}
	name := args[0]
	command, ok := m.Commands[name]
	if !ok {
		if msg, isTopic := m.tryImplicitTopic(args); isTopic {
			fmt.Fprint(m.stdout, msg)
			return
		}

		topicBasedName := m.findTopicBasedCommand(args)

		fmt.Fprintf(m.stderr, "%s: %q is not a %s command. See %q.\n", m.name, topicBasedName, m.name, m.name+" help")
		var keys []string
		for key := range m.Commands {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		possibleCommands := []string{}

		for _, key := range keys {
			topicBasedKey := strings.ReplaceAll(key, "-", " ")

			if len(args) == 1 && strings.Contains(topicBasedKey, args[0]) {
				possibleCommands = append(possibleCommands, topicBasedKey)
				continue
			}

			if score := fuzzy.Levenshtein(&topicBasedKey, &topicBasedName); score < 3 {
				possibleCommands = append(possibleCommands, topicBasedKey)
			}
		}
		if len(possibleCommands) > 0 {
			fmt.Fprintln(m.stderr, "\nDid you mean?")

			for _, cmd := range possibleCommands {
				fmt.Fprintf(m.stderr, "\t%s\n", cmd)
			}
		}
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
	client := NewClient(net.Dial15FullUnlimitedClient, context, m)
	client.Verbosity = verbosity
	sigChan := make(chan os.Signal, 1)
	if cancelable, ok := command.(Cancelable); ok {
		signal.Notify(sigChan, syscall.SIGINT)
		go func(context Context, client *Client) {
			for range sigChan {
				fmt.Fprintln(m.stdout, "Attempting command cancellation...")
				errCancel := cancelable.Cancel(context, client)
				if errCancel == nil {
					return
				}
				fmt.Fprintf(m.stderr, "Error canceling command: %v. Proceeding.", errCancel)
			}
		}(*context, client)
	}
	err = command.Run(context, client)
	close(sigChan)
	if isUnauthorized(err) && name != loginCmdName {
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
		if verbosity > 0 {
			errorMsg = fmt.Sprintf("%+v", err)
		}
		if bodyErr, ok := err.(interface {
			Body() []byte
		}); ok {
			body := string(bodyErr.Body())
			if body != "" {
				errorMsg = fmt.Sprintf("%s: %s", errorMsg, body)
			}
		}
		if isUnauthorized(err) && name != loginCmdName {
			errorMsg = fmt.Sprintf(`You're not authenticated or your session has expired. Please use %q command for authentication.`, loginCmdName)
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

func (m *Manager) findTopicBasedCommand(args []string) string {
	safeCmd := []string{}
	for _, arg := range args {
		if len(arg) > 0 && arg[0] == '-' {
			break
		}

		safeCmd = append(safeCmd, arg)
	}

	return strings.Join(safeCmd, " ")
}

func (m *Manager) newContext(args []string, stdout io.Writer, stderr io.Writer, stdin io.Reader) *Context {
	stdout = newPagerWriter(stdout)
	stdin = newSyncReader(stdin, stdout)
	ctx := &Context{Args: args, Stdout: stdout, Stderr: stderr, Stdin: stdin}
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

var topicRE = regexp.MustCompile(`(?s)^(.*)\n*$`)

func (m *Manager) tryImplicitTopic(args []string) (string, bool) {
	topicName := strings.Join(args, " ")

	topic, isExplicit := m.topics[topicName]
	commands := m.topicCommands[topicName]

	if len(commands) == 0 && !isExplicit {
		return "", false
	}

	if len(commands) > 0 {
		if len(topic) > 0 {
			topic = topicRE.ReplaceAllString(topic, "$1\n\n")
		}

		topic += fmt.Sprintf("The following commands are available in the %q topic:\n\n", topicName)
		var group []string
		for _, command := range commands {
			group = append(group, command.Info().Name)
		}
		topic += m.dumpCommands(group)
	}

	return topic, true
}

func formatDescriptionLine(label, description string, maxSize int) string {
	description = strings.Split(description, "\n")[0]
	description = strings.Split(description, ".")[0]
	if len(description) > 2 {
		description = strings.ToUpper(description[:1]) + description[1:]
	}
	fmtStr := fmt.Sprintf("  %%-%ds %%s\n", maxSize)
	return fmt.Sprintf(fmtStr, strings.ReplaceAll(label, "-", " "), description)
}

func maxLabelSize(labels []string) int {
	maxLabelSize := 20
	for _, label := range labels {
		if len(label) > maxLabelSize {
			maxLabelSize = len(label)
		}
	}
	return maxLabelSize
}

func (m *Manager) dumpCommands(commands []string) string {
	sort.Strings(commands)
	var output string
	maxCmdSize := maxLabelSize(commands)
	for _, command := range commands {
		output += formatDescriptionLine(command, m.Commands[command].Info().Desc, maxCmdSize)
	}
	output += fmt.Sprintf("\nUse %s help <commandname> to get more information about a command.\n", m.name)
	return output
}

func (m *Manager) dumpTopics() string {
	topics := m.discoverTopics()
	sort.Strings(topics)
	maxTopicSize := maxLabelSize(topics)
	var output string
	for _, topic := range topics {
		output += formatDescriptionLine(topic, m.topics[topic], maxTopicSize)
	}
	output += fmt.Sprintf("\nUse %s help <topicname> to get more information about a topic.\n", m.name)
	return output
}

func (m *Manager) normalizeCommandArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	newArgs := append([]string{}, args...)
	for len(newArgs) > 0 {
		tryCmd := strings.Join(newArgs, "-")
		if _, ok := m.Commands[tryCmd]; ok {
			break
		}
		newArgs = newArgs[:len(newArgs)-1]
	}
	remainder := len(newArgs)
	if remainder > 0 {
		newArgs = []string{strings.Join(newArgs, "-")}
	}
	newArgs = append(newArgs, args[remainder:]...)
	return newArgs
}

func (m *Manager) discoverTopics() []string {
	freq := map[string]int{}
	for cmdName, cmd := range m.Commands {
		if _, isDeprecated := cmd.(*DeprecatedCommand); isDeprecated {
			continue
		}
		idx := strings.Index(cmdName, "-")
		if idx != -1 {
			freq[cmdName[:idx]] += 1
		}
	}
	for topic := range m.topics {
		freq[topic] = 999
	}
	var result []string
	for topic, count := range freq {
		if count > 1 {
			result = append(result, topic)
		}
	}
	sort.Strings(result)
	return result
}

// Cancelable are implemented by commands that support cancellation
type Cancelable interface {
	// Cancel handles the command cancellation and is required to be thread safe as
	// this method is called by a different goroutine.
	// Cancel should return an error if the operation is not cancelable yet/anymore or there
	// was any error during the cancellation.
	// Cancel may be called multiple times.
	Cancel(context Context, client *Client) error
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

type help struct {
	manager *Manager
}

func (c *help) Info() *Info {
	return &Info{Name: "help", Usage: "command [args]"}
}

func (c *help) Run(context *Context, client *Client) error {
	const deprecatedMsg = "WARNING: %q is deprecated. Showing help for %q instead.\n\n"
	output := fmt.Sprintf("%s\n", versionString(c.manager))
	if c.manager.wrong {
		output += "ERROR: wrong number of arguments.\n\n"
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
			output += "\n"
		} else if msg, ok := c.manager.tryImplicitTopic(context.Args); ok {
			output += msg
		} else {
			return errors.Errorf("command %q does not exist.", context.Args[0])
		}
	} else {
		output += fmt.Sprintf("Usage: %s %s\n\nAvailable commands:\n", c.manager.name, c.Info().Usage)
		var commands []string
		for name, cmd := range c.manager.Commands {
			if _, ok := cmd.(*DeprecatedCommand); !ok {
				commands = append(commands, name)
			}
		}
		output += c.manager.dumpCommands(commands)
		if len(c.manager.topics) > 0 {
			output += fmt.Sprintln("\nAvailable topics:")
			output += c.manager.dumpTopics()
		}
	}
	io.WriteString(context.Stdout, output)
	return nil
}

var flagFormatRegexp = regexp.MustCompile(`(?m)^([^-\s])`)

func (c *help) parseFlags(command Command) string {
	var output string
	if cmd, ok := command.(FlaggedCommand); ok {
		var buf bytes.Buffer
		flagset := cmd.Flags()
		flagset.SetOutput(&buf)
		flagset.PrintDefaults()
		if buf.String() != "" {
			output = flagFormatRegexp.ReplaceAllString(buf.String(), `    $1`)
			output = fmt.Sprintf("Flags:\n\n%s", output)
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

var GitHash = ""

func versionString(manager *Manager) string {
	suffix := "\n"
	if GitHash != "" {
		suffix = fmt.Sprintf(" hash %s\n", GitHash)
	}
	return fmt.Sprintf("Client version: %s.%s", manager.version, suffix)
}

func apiVersionString(client *Client) (string, error) {
	if client == nil {
		return "", fmt.Errorf("client cannot be nil")
	}

	url, err := GetURL("/info")
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		// if we return the error, stdout won't flush until the prompt
		return fmt.Sprintf("Unable to retrieve server version: %v", err), nil
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var version map[string]string
	err = json.Unmarshal(body, &version)
	if err != nil {
		return "", err
	}

	resp.Body.Close()
	return fmt.Sprintf("Server version: %s.\n", version["version"]), nil
}

func (c *version) Run(context *Context, client *Client) error {
	fmt.Fprint(context.Stdout, versionString(c.manager))

	apiVersion, err := apiVersionString(client)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, apiVersion)

	return nil
}

func ExtractProgramName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

var (
	fsystem   fs.Fs
	fsystemMu sync.Mutex
)

func filesystem() fs.Fs {
	fsystemMu.Lock()
	defer fsystemMu.Unlock()
	if fsystem == nil {
		fsystem = fs.OsFs{}
	}
	return fsystem
}

// validateVersion checks whether current version is greater or equal to
// supported version.
func validateVersion(supported, current string) bool {
	if supported == "" {
		return true
	}
	vSupported, err := goVersion.NewVersion(supported)
	if err != nil {
		return false
	}
	vCurrent, err := goVersion.NewVersion(current)
	if err != nil {
		return false
	}
	return vCurrent.Compare(vSupported) >= 0
}
