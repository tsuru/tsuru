package cmd

import "fmt"

type Manager struct {
	commands map[string]Command
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
	command := m.commands[args[0]]
	command.Run()
}

type Command interface {
	Run() error
	Info() *Info
}

type Info struct {
	Name string
}
