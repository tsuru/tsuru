// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/repository"
)

// gitDeployCmds returns the list of commands that are used when the
// provisioner deploys a unit using the Git repository method.
func gitDeployCmds(app provision.App, version string) ([]string, error) {
	repo, err := repository.Manager().GetRepository(app.GetName())
	if err != nil {
		return nil, err
	}
	return deployCmds(app, "git", repo.ReadOnlyURL, version)
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
	cmds := append([]string{deployCmd}, params...)
	host, _ := config.GetString("host")
	token := app.Envs()["TSURU_APP_TOKEN"].Value
	unitAgentCmds := []string{"tsuru_unit_agent", host, token, app.GetName(), `"` + strings.Join(cmds, " ") + `"`, "deploy"}
	finalCmd := strings.Join(unitAgentCmds, " ")
	return []string{"/bin/bash", "-lc", finalCmd}, nil
}

// runWithAgentCmds returns the list of commands that should be passed when the
// provisioner will run a unit using tsuru_unit_agent to start.
//
// This will only be called for legacy containers that have not been re-
// deployed since the introduction of independent units per 'process' in
// 0.12.0.
func runWithAgentCmds(app provision.App) ([]string, error) {
	runCmd, err := config.GetString("docker:run-cmd:bin")
	if err != nil {
		return nil, err
	}
	host, _ := config.GetString("host")
	token := app.Envs()["TSURU_APP_TOKEN"].Value
	return []string{"tsuru_unit_agent", host, token, app.GetName(), runCmd}, nil
}

func processCmdForImage(processName, imageId string) (string, string, error) {
	data, err := getImageCustomData(imageId)
	if err != nil {
		return "", "", err
	}
	if processName == "" {
		if len(data.Processes) == 0 {
			return "", "", nil
		}
		if len(data.Processes) > 1 {
			return "", "", provision.InvalidProcessError{Msg: "no process name specified and more than one declared in Procfile"}
		}
		for name := range data.Processes {
			processName = name
		}
	}
	processCmd := data.Processes[processName]
	if processCmd == "" {
		return "", "", provision.InvalidProcessError{Msg: fmt.Sprintf("no command declared in Procfile for process %q", processName)}
	}
	return processCmd, processName, nil
}

func runLeanContainerCmds(processName, imageId string, app provision.App) ([]string, string, error) {
	processCmd, processName, err := processCmdForImage(processName, imageId)
	if err != nil {
		return nil, "", err
	}
	if processCmd == "" {
		// Legacy support, no processes are yet registered for this app's
		// containers.
		cmds, err := runWithAgentCmds(app)
		return cmds, "", err
	}
	yamlData, err := getImageTsuruYamlData(imageId)
	if err != nil {
		return nil, "", err
	}
	before := strings.Join(yamlData.Hooks.Restart.Before, " && ")
	if before != "" {
		before += " && "
	}
	return []string{
		"/bin/bash",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; " + before + "exec " + processCmd,
	}, processName, nil
}
