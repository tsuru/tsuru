// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/repository"
)

// gitDeployCmds returns the list of commands that are used when the
// provisioner deploys a unit using the Git repository method.
func gitDeployCmds(app provision.App, version string) ([]string, error) {
	appRepo := repository.ReadOnlyURL(app.GetName())
	return deployCmds(app, "git", appRepo, version)
}

// archiveDeployCmds returns the list of commands that are used when the
// provisioner deploys a unit using the archive method.
func archiveDeployCmds(app provision.App, archiveURL string) ([]string, error) {
	return deployCmds(app, "archive", archiveURL)
}

func deployCmds(app provision.App, params ...string) ([]string, error) {
	deployCmd, err := config.GetString("docker:deploy-cmd")
	if err != nil {
		return nil, err
	}
	// This method is overly complicated for us to be able to support platform
	// images with old versions of tsuru-unit-agent. After 0.8.0 we'll
	// probably drop support for old images simplifying this function.
	var envs string
	for _, env := range app.Envs() {
		envs += fmt.Sprintf(`%s=%s `, env.Name, strings.Replace(env.Value, " ", "", -1))
	}
	cmds := append([]string{deployCmd}, params...)
	cmdsWithEnv := strings.Join(append(cmds, envs), " ")
	host := app.Envs()["TSURU_HOST"].Value
	token := app.Envs()["TSURU_APP_TOKEN"].Value
	unitAgentCmds := []string{"tsuru_unit_agent", host, token, app.GetName(), `"` + strings.Join(cmds, " ") + `"`, "deploy"}
	alternatives := fmt.Sprintf("%s || %s", strings.Join(unitAgentCmds, " "), cmdsWithEnv)
	return []string{"/bin/bash", "-c", alternatives}, nil
}

// runWithAgentCmds returns the list of commands that should be passed when the
// provisioner will run a unit using tsuru_unit_agent to start.
func runWithAgentCmds(app provision.App, publicKey []byte) ([]string, error) {
	runCmd, err := config.GetString("docker:run-cmd:bin")
	if err != nil {
		return nil, err
	}
	ssh, err := sshCmds(publicKey)
	if err != nil {
		return nil, err
	}
	host := app.Envs()["TSURU_HOST"].Value
	token := app.Envs()["TSURU_APP_TOKEN"].Value
	unitAgentCmds := []string{"tsuru_unit_agent", host, token, app.GetName(), runCmd}
	unitAgentCmd := strings.Join(unitAgentCmds, " ")
	sshCmd := strings.Join(ssh, " && ")
	cmd := fmt.Sprintf("%s && %s", unitAgentCmd, sshCmd)
	cmds := []string{"/bin/bash", "-c", cmd}
	return cmds, nil
}

// sshCmds returns the commands needed to start a ssh daemon.
func sshCmds(publicKey []byte) ([]string, error) {
	addKeyCommand, err := config.GetString("docker:ssh:add-key-cmd")
	if err != nil {
		return nil, err
	}
	sshdCommand, err := config.GetString("docker:ssh:sshd-path")
	if err != nil {
		sshdCommand = "sudo /usr/sbin/sshd"
	}
	return []string{
		fmt.Sprintf("%s %s", addKeyCommand, bytes.TrimSpace(publicKey)),
		sshdCommand + " -D",
	}, nil
}
