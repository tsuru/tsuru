package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
)

type Manager struct {
	commands map[string]interface{}
	Stdout   io.Writer
	Stderr   io.Writer
}

func (m *Manager) Register(command interface{}) {
	if m.commands == nil {
		m.commands = make(map[string]interface{})
	}
	name := command.(SimpleCommand).Info().Name
	_, found := m.commands[name]
	if found {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	m.commands[name] = command
}

func (m *Manager) Run(args []string) {
	command, exist := m.commands[args[0]]
	if !exist {
		io.WriteString(m.Stderr, fmt.Sprintf("command %s does not exist\n", args[0]))
		return
	}
	switch command.(type) {
	case Command:
		if len(args) > 1 {
			args = args[1:]
		}
		subcommand, exist := command.(Command).Subcommands()[args[0]]
		if exist {
			command = subcommand
		}
	}
	err := command.(SimpleCommand).Run(&Context{args[1:], m.Stdout, m.Stderr}, NewClient(&http.Client{}))
	if err != nil {
		io.WriteString(m.Stderr, err.Error())
	}
}

func NewManager(stdout, stderr io.Writer) Manager {
	return Manager{Stdout: stdout, Stderr: stderr}
}

type Command interface {
	Run(context *Context, client Doer) error
	Info() *Info
	Subcommands() map[string]Command
}

type SimpleCommand interface {
	Run(context *Context, client Doer) error
	Info() *Info
}

type Context struct {
	Args   []string
	Stdout io.Writer
	Stderr io.Writer
}

type Info struct {
	Name string
}

func WriteToken(token string) error {
	user, err := user.Current()
	tokenPath := user.HomeDir + "/.tsuru_token"
	file, err := os.Create(tokenPath)
	if err != nil {
		return err
	}
	_, err = file.WriteString(token)
	if err != nil {
		return err
	}
	return nil
}

func ReadToken() (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	tokenPath := user.HomeDir + "/.tsuru_token"
	token, err := ioutil.ReadFile(tokenPath)
	if err != nil {
		return "", err
	}
	return string(token), nil
}
