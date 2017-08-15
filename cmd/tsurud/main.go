// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"
	"os"

	"github.com/docker/machine/libmachine/drivers/plugin/localbinary"
	"github.com/google/gops/agent"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api"
	_ "github.com/tsuru/tsuru/builder/docker"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/iaas/dockermachine"
	_ "github.com/tsuru/tsuru/provision/docker"
	_ "github.com/tsuru/tsuru/provision/kubernetes"
	_ "github.com/tsuru/tsuru/provision/swarm"
	_ "github.com/tsuru/tsuru/repository/gandalf"
	_ "github.com/tsuru/tsuru/storage/mongodb"
)

const defaultConfigPath = "/etc/tsuru/tsuru.conf"

var configPath = defaultConfigPath

func buildManager() *cmd.Manager {
	m := cmd.NewManager("tsurud", api.Version, "", os.Stdout, os.Stderr, os.Stdin, nil)
	m.Register(&tsurudCommand{Command: &apiCmd{}})
	m.Register(&tsurudCommand{Command: tokenCmd{}})
	m.Register(&tsurudCommand{Command: &migrateCmd{}})
	m.Register(&tsurudCommand{Command: gandalfSyncCmd{}})
	m.Register(&tsurudCommand{Command: createRootUserCmd{}})
	m.Register(&migrationListCmd{})
	return m
}

func inDockerMachineDriverMode() bool {
	return os.Getenv(localbinary.PluginEnvKey) == localbinary.PluginEnvVal
}

func main() {
	err := agent.Listen(&agent.Options{
		NoShutdownCleanup: true,
	})
	if err != nil {
		log.Fatalf("Unable to start a Gops agent %s", err)
	}
	defer agent.Close()
	if inDockerMachineDriverMode() {
		err := dockermachine.RunDriver(os.Getenv(localbinary.PluginEnvDriverName))
		if err != nil {
			log.Fatalf("Error running driver: %s", err)
		}
	} else {
		localbinary.CurrentBinaryIsDockerMachine = true
		config.ReadConfigFile(configPath)
		listenSignals()
		m := buildManager()
		m.Run(os.Args[1:])
	}
}
