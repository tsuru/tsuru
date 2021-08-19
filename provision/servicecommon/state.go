// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"context"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
)

func ChangeAppState(ctx context.Context, manager ServiceManager, a provision.App, process string, state ProcessState, version appTypes.AppVersion) error {
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
	err = RunServicePipeline(ctx, manager, version.Version(), provision.DeployArgs{App: a, Version: version, PreserveVersions: true}, spec)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func ChangeUnits(ctx context.Context, manager ServiceManager, a provision.App, units int, processName string, version appTypes.AppVersion) error {
	if a.GetDeploys() == 0 {
		return errors.New("units can only be modified after the first deploy")
	}
	err := RunServicePipeline(ctx, manager, version.Version(), provision.DeployArgs{App: a, Version: version, PreserveVersions: true}, ProcessSpec{
		processName: ProcessState{Increment: units, Start: true},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
