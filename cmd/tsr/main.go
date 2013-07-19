// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/provision"
	_ "github.com/globocom/tsuru/provision/docker"
	_ "github.com/globocom/tsuru/provision/juju"
	"os"
)

func buildManager() *cmd.Manager {
	m := cmd.NewManager("tsr", "0.1.0", "", os.Stdout, os.Stderr, os.Stdin)
	m.Register(&tsrCommand{Command: &apiCmd{}})
	m.Register(&tsrCommand{Command: &collectorCmd{}})
	m.Register(&tsrCommand{Command: tokenCmd{}})
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
	m := buildManager()
	m.Run(os.Args[1:])
}
