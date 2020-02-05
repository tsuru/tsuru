// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/servicemanager"
)

func ChangeAppState(manager ServiceManager, a provision.App, process string, state ProcessState) error {
	var (
		processes []string
		err       error
	)
	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(a)
	if err != nil {
		return err
	}
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
	err = RunServicePipeline(manager, a, version, spec, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func ChangeUnits(manager ServiceManager, a provision.App, units int, processName string) error {
	if a.GetDeploys() == 0 {
		return errors.New("units can only be modified after the first deploy")
	}
	if units == 0 {
		return errors.New("cannot change 0 units")
	}
	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(a)
	if err != nil {
		return err
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
	err = RunServicePipeline(manager, a, version, ProcessSpec{
		processName: ProcessState{Increment: units},
	}, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
