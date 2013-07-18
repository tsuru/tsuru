// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/log"
	_ "github.com/globocom/tsuru/provision/docker"
	_ "github.com/globocom/tsuru/provision/juju"
	"launchpad.net/gnuflag"
	"os"
)

var configFile string

func init() {
	var buf bytes.Buffer
	fs := gnuflag.NewFlagSet("tsr", gnuflag.ContinueOnError)
	fs.SetOutput(&buf)
	fs.StringVar(&configFile, "config", "/etc/tsuru/tsuru.conf", "tsr config file.")
	fs.StringVar(&configFile, "c", "/etc/tsuru/tsuru.conf", "tsr config file.")
	fs.Parse(true, os.Args[1:])
}

func buildManager() *cmd.Manager {
	err := config.ReadAndWatchConfigFile(configFile)
	if err != nil {
		log.Fatal(err)
	}
	m := cmd.NewManager("tsr", "0.1.0", "", os.Stdout, os.Stderr, os.Stdin)
	m.Register(&apiCmd{})
	m.Register(&collectorCmd{})
	m.Register(&tokenCmd{})
	return m
}

func main() {
	m := buildManager()
	m.Run(os.Args[1:])
}
