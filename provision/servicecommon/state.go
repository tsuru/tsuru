// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
)

func ChangeAppState(manager ServiceManager, a provision.App, process string, state ProcessState) error {
	var (
		processes []string
		err       error
	)
	if process == "" {
		processes, err = image.AllAppProcesses(a.GetName())
		if err != nil {
			return err
		}
	} else {
		processes = []string{process}
	}
	imageName, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return errors.WithStack(err)
	}
	spec := ProcessSpec{}
	for _, procName := range processes {
		spec[procName] = state
	}
	err = RunServicePipeline(manager, a, imageName, spec, nil)
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
	imageName, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return err
	}
	if processName == "" {
		_, processName, err = dockercommon.ProcessCmdForImage(processName, imageName)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	err = RunServicePipeline(manager, a, imageName, ProcessSpec{
		processName: ProcessState{Increment: units},
	}, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
