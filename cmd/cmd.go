package cmd

import (
	"fmt"
	"io"
)

type Manager struct {
	commands map[string]Command
	Stdout   io.Writer
	Stderr   io.Writer
}

func (m *Manager) Register(command Command) {
	if m.commands == nil {
		m.commands = make(map[string]Command)
	}
	name := command.Info().Name
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
	command.Run()
}

func NewManager(stdout, stderr io.Writer) Manager {
	return Manager{Stdout: stdout, Stderr: stderr}
}

type Command interface {
	Run() error
	Info() *Info
}

type Info struct {
	Name string
}
