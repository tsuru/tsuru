// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockercommon

import (
	"fmt"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
)

// ArchiveBuildCmds build a image using the archive method.
func ArchiveBuildCmds(app provision.App, archiveURL string) []string {
	return buildCmds(app, "build", "archive", archiveURL)
}

// ArchiveDeployCmds is a legacy command to deploys an unit using the archive method.
func ArchiveDeployCmds(app provision.App, archiveURL string) []string {
	return buildCmds(app, "deploy", "archive", archiveURL)
}

// DeployCmds deploys an unit builded by tsuru.
func DeployCmds(app provision.App) []string {
	uaCmds := unitAgentCmds(app)
	uaCmds = append(uaCmds, "deploy-only")
	finalCmd := strings.Join(uaCmds, " ")
	return []string{"/bin/sh", "-lc", finalCmd}
}

func buildCmds(app provision.App, agentCmd string, params ...string) []string {
	deployCmd, err := config.GetString("docker:deploy-cmd")
	if err != nil {
		deployCmd = "/var/lib/tsuru/deploy"
	}
	cmds := append([]string{deployCmd}, params...)
	uaCmds := unitAgentCmds(app)
	uaCmds = append(uaCmds, `"`+strings.Join(cmds, " ")+`"`, agentCmd)
	finalCmd := strings.Join(uaCmds, " ")
	return []string{"/bin/sh", "-lc", finalCmd}
}

func unitAgentCmds(app provision.App) []string {
	host, _ := config.GetString("host")
	token := app.Envs()["TSURU_APP_TOKEN"].Value
	return []string{"tsuru_unit_agent", host, token, app.GetName()}
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
		runCmd = "/var/lib/tsuru/start"
	}
	host, _ := config.GetString("host")
	token := app.Envs()["TSURU_APP_TOKEN"].Value
	return []string{"tsuru_unit_agent", host, token, app.GetName(), runCmd}, nil
}

func ProcessCmdForImage(processName, imageID string) ([]string, string, error) {
	data, err := image.GetImageMetaData(imageID)
	if err != nil {
		return nil, "", err
	}
	if processName == "" {
		if len(data.Processes) == 0 {
			return nil, "", nil
		}
		if len(data.Processes) > 1 {
			return nil, "", provision.InvalidProcessError{Msg: "no process name specified and more than one declared in Procfile"}
		}
		for name := range data.Processes {
			processName = name
		}
	}
	processCmd := data.Processes[processName]
	if len(processCmd) == 0 {
		return nil, "", provision.InvalidProcessError{Msg: fmt.Sprintf("no command declared in Procfile for process %q", processName)}
	}
	return processCmd, processName, nil
}

func LeanContainerCmds(processName, imageID string, app provision.App) ([]string, string, error) {
	return LeanContainerCmdsWithExtra(processName, imageID, app, nil)
}

func LeanContainerCmdsWithExtra(processName, imageID string, app provision.App, extraCmds []string) ([]string, string, error) {
	processCmd, processName, err := ProcessCmdForImage(processName, imageID)
	if err != nil {
		return nil, "", err
	}
	if len(processCmd) == 0 {
		// Legacy support, no processes are yet registered for this app's
		// containers.
		var cmds []string
		cmds, err = runWithAgentCmds(app)
		return cmds, "", err
	}
	yamlData, err := image.GetImageTsuruYamlData(imageID)
	if err != nil {
		return nil, "", err
	}
	if yamlData.Hooks != nil {
		extraCmds = append(extraCmds, yamlData.Hooks.Restart.Before...)
	}
	before := strings.Join(extraCmds, " && ")
	if before != "" {
		before += " && "
	}
	if processName == "" {
		processName = "web"
	}
	allCmds := []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; " + before,
	}
	if len(processCmd) > 1 {
		allCmds[len(allCmds)-1] += "exec $0 \"$@\""
		allCmds = append(allCmds, processCmd...)
	} else {
		allCmds[len(allCmds)-1] += "exec " + processCmd[0]
	}
	return allCmds, processName, nil
}
