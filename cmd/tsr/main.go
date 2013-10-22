// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/provision"
	_ "github.com/globocom/tsuru/provision/docker"
	_ "github.com/globocom/tsuru/provision/juju"
	"os"
)

const defaultConfigPath = "/etc/tsuru/tsuru.conf"

func buildManager() *cmd.Manager {
	m := cmd.NewManager("tsr", "0.2.4.1", "", os.Stdout, os.Stderr, os.Stdin)
	m.Register(&tsrCommand{Command: &apiCmd{}})
	m.Register(&tsrCommand{Command: &collectorCmd{}})
	m.Register(&tsrCommand{Command: tokenCmd{}})
	m.Register(&tsrCommand{Command: &healerCmd{}})
	registerProvisionersCommands(m)
	return m
}

func registerProvisionersCommands(m *cmd.Manager) {
	provisioners := provision.Registry()
	for _, p := range provisioners {
		if c, ok := p.(provision.Commandable); ok {
			commands := c.Commands()
			for _, cmd := range commands {
				m.Register(&tsrCommand{Command: cmd})
			}
		}
	}
}

func main() {
	config.ReadConfigFile(defaultConfigPath)
	m := buildManager()
	m.Run(os.Args[1:])
}
