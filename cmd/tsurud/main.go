// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/provision"
	_ "github.com/tsuru/tsuru/provision/docker"
	_ "github.com/tsuru/tsuru/repository/gandalf"
)

const defaultConfigPath = "/etc/tsuru/tsuru.conf"

var configPath = defaultConfigPath

func buildManager() *cmd.Manager {
	m := cmd.NewManager("tsurud", api.Version, "", os.Stdout, os.Stderr, os.Stdin, nil)
	m.Register(&tsurudCommand{Command: &apiCmd{}})
	m.Register(&tsurudCommand{Command: tokenCmd{}})
	m.Register(&tsurudCommand{Command: &migrateCmd{}})
	m.Register(&tsurudCommand{Command: gandalfSyncCmd{}})
	registerProvisionersCommands(m)
	return m
}

func registerProvisionersCommands(m *cmd.Manager) {
	provisioners := provision.Registry()
	for _, p := range provisioners {
		if c, ok := p.(cmd.Commandable); ok {
			commands := c.Commands()
			for _, cmd := range commands {
				m.Register(&tsurudCommand{Command: cmd})
			}
		}
	}
}

func listenSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	go func() {
		for range ch {
			config.ReadConfigFile(configPath)
		}
	}()
}

func main() {
	config.ReadConfigFile(configPath)
	listenSignals()
	m := buildManager()
	m.Run(os.Args[1:])
}
