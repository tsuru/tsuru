// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	appTypes "github.com/tsuru/tsuru/types/app"
)

func ChangeAppState(manager ServiceManager, a provision.App, process string, state ProcessState, version appTypes.AppVersion) error {
	var (
		processes []string
		err       error
	)
	if process == "" {
		var allProcesses map[string][]string
		allProcesses, err = version.Processes()
		if err != nil {
			return err
		}
		for processName := range allProcesses {
			processes = append(processes, processName)
		}
	} else {
		processes = []string{process}
	}
	spec := ProcessSpec{}
	for _, procName := range processes {
		spec[procName] = state
	}
	err = RunServicePipeline(manager, version.Version(), provision.DeployArgs{App: a, Version: version, PreserveVersions: true}, spec)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func ChangeUnits(manager ServiceManager, a provision.App, units int, processName string, version appTypes.AppVersion) error {
	if a.GetDeploys() == 0 {
		return errors.New("units can only be modified after the first deploy")
	}
	if units == 0 {
		return errors.New("cannot change 0 units")
	}
	cmdData, err := dockercommon.ContainerCmdsDataFromVersion(version)
	if err != nil {
		return err
	}
	if processName == "" {
		_, processName, err = dockercommon.ProcessCmdForVersion(processName, cmdData)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	err = RunServicePipeline(manager, version.Version(), provision.DeployArgs{App: a, Version: version, PreserveVersions: true}, ProcessSpec{
		processName: ProcessState{Increment: units},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
