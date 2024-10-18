// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockercommon

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

type ContainerCmdsData struct {
	yamlData  provTypes.TsuruYamlData
	processes map[string][]string
}

func ContainerCmdsDataFromVersion(version appTypes.AppVersion) (ContainerCmdsData, error) {
	var cmdData ContainerCmdsData
	var err error
	cmdData.yamlData, err = version.TsuruYamlData()
	if err != nil {
		return cmdData, err
	}
	cmdData.processes, err = version.Processes()
	if err != nil {
		return cmdData, err
	}
	return cmdData, nil
}

func ProcessCmdForVersion(processName string, cmdData ContainerCmdsData) ([]string, string, error) {
	if processName == "" {
		if len(cmdData.processes) == 0 {
			return nil, "", nil
		}
		if len(cmdData.processes) > 1 {
			return nil, "", provision.InvalidProcessError{Msg: "no process name specified and more than one declared in Procfile"}
		}
		for name := range cmdData.processes {
			processName = name
		}
	}
	processCmd := cmdData.processes[processName]
	if len(processCmd) == 0 {
		return nil, "", provision.InvalidProcessError{Msg: fmt.Sprintf("no command declared in Procfile for process %q", processName)}
	}
	return processCmd, processName, nil
}

func LeanContainerCmds(processName string, cmdData ContainerCmdsData, app *appTypes.App) ([]string, string, error) {
	processCmd, processName, err := ProcessCmdForVersion(processName, cmdData)
	if err != nil {
		return nil, "", err
	}
	if len(processCmd) == 0 {
		return nil, "", errors.New("Legacy support of app's container are deprecated")
	}
	var extraCmds []string
	if cmdData.yamlData.Hooks != nil {
		extraCmds = append(extraCmds, cmdData.yamlData.Hooks.Restart.Before...)
	}
	before := strings.Join(extraCmds, " && ")
	if before != "" {
		before += " && "
	}
	if processName == "" {
		processName = provision.WebProcessName
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
