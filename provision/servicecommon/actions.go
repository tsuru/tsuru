// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type ProcessState struct {
	Stop      bool
	Start     bool
	Restart   bool
	Sleep     bool
	Increment int
}

type ProcessSpec map[string]ProcessState

type pipelineArgs struct {
	manager          ServiceManager
	app              provision.App
	oldVersion       appTypes.AppVersion
	newVersion       appTypes.AppVersion
	newVersionSpec   ProcessSpec
	event            *event.Event
	preserveVersions bool
}

type labelReplicas struct {
	labels       *provision.LabelSet
	realReplicas int
}

type ServiceManager interface {
	RemoveService(a provision.App, processName string, version appTypes.AppVersion) error
	CurrentLabels(a provision.App, processName string, version appTypes.AppVersion) (*provision.LabelSet, error)
	DeployService(ctx context.Context, a provision.App, processName string, labels *provision.LabelSet, replicas int, version appTypes.AppVersion, preserveVersions bool) error
	CleanupServices(a provision.App, version appTypes.AppVersion) error
}

func RunServicePipeline(manager ServiceManager, oldVersion appTypes.AppVersion, args provision.DeployArgs, updateSpec ProcessSpec) error {
	newProcesses, err := args.Version.Processes()
	if err != nil {
		return err
	}
	if len(newProcesses) == 0 {
		return errors.Errorf("no process information found deploying version %q", args.Version)
	}
	newSpec := ProcessSpec{}
	for p := range newProcesses {
		newSpec[p] = ProcessState{Start: true}
		if updateSpec != nil {
			newSpec[p] = updateSpec[p]
		}
	}
	pipeline := action.NewPipeline(
		updateServices,
		updateImageInDB,
		removeOldServices,
	)
	return pipeline.Execute(&pipelineArgs{
		manager:          manager,
		app:              args.App,
		preserveVersions: args.PreserveVersions,
		oldVersion:       oldVersion,
		newVersion:       args.Version,
		newVersionSpec:   newSpec,
		event:            args.Event,
	})
}

func rollbackAddedProcesses(args *pipelineArgs, processes map[string]*labelReplicas) {
	for processName, oldLabels := range processes {
		var err error

		if args.oldVersion == nil || oldLabels.labels == nil {
			err = args.manager.RemoveService(args.app, processName, args.newVersion)
		} else {
			err = args.manager.DeployService(context.Background(), args.app, processName, oldLabels.labels, oldLabels.realReplicas, args.oldVersion, args.preserveVersions)
		}

		if err != nil {
			log.Errorf("error rolling back updated service for %s[%s]: %+v", args.app.GetName(), processName, err)
		}
	}
}

func rawLabelsAndReplicas(args *pipelineArgs, processName string, version appTypes.AppVersion) (*labelReplicas, error) {
	if version == nil {
		return &labelReplicas{}, nil
	}
	labels, err := args.manager.CurrentLabels(args.app, processName, version)
	if err != nil {
		return nil, err
	}
	if labels == nil {
		return &labelReplicas{}, nil
	}
	replicas := labels.AppReplicas()
	isStopped := labels.IsStopped()
	realReplicas := replicas
	if isStopped {
		realReplicas = 0
	}
	return &labelReplicas{labels: labels, realReplicas: realReplicas}, nil
}

func labelsForService(args *pipelineArgs, oldLabels labelReplicas, newVersion appTypes.AppVersion, processName string, pState ProcessState) (labelReplicas, error) {
	replicas := 0
	restartCount := 0
	isStopped := false
	isAsleep := false
	if oldLabels.labels != nil {
		replicas = oldLabels.labels.AppReplicas()
		restartCount = oldLabels.labels.Restarts()
		isStopped = oldLabels.labels.IsStopped()
		isAsleep = oldLabels.labels.IsAsleep()
	}
	if pState.Increment != 0 {
		replicas += pState.Increment
		if replicas < 0 {
			return oldLabels, errors.New("cannot have less than 0 units")
		}
	}
	if pState.Start || pState.Restart {
		if replicas == 0 {
			replicas = 1
		}
		isStopped = false
		isAsleep = false
	}
	labels, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      args.app,
		Process:  processName,
		Replicas: replicas,
		Version:  newVersion.Version(),
	})
	if err != nil {
		return oldLabels, err
	}
	oldLabels.realReplicas = replicas
	if isStopped || pState.Stop {
		oldLabels.realReplicas = 0
		labels.SetStopped()
	}
	if isAsleep || pState.Sleep {
		labels.SetAsleep()
	}
	if pState.Restart {
		restartCount++
		labels.SetRestarts(restartCount)
	}
	oldLabels.labels = labels
	return oldLabels, nil
}

var updateServices = &action.Action{
	Name: "update-services",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(*pipelineArgs)
		var toDeployProcesses []string
		deployedProcesses := map[string]*labelReplicas{}

		for processName := range args.newVersionSpec {
			toDeployProcesses = append(toDeployProcesses, processName)
		}
		sort.Strings(toDeployProcesses)
		totalUnits := 0
		oldLabelsMap := map[string]*labelReplicas{}
		newLabelsMap := map[string]*labelReplicas{}
		for _, processName := range toDeployProcesses {
			oldLabels, err := rawLabelsAndReplicas(args, processName, args.oldVersion)
			if err != nil {
				return nil, err
			}
			oldLabelsMap[processName] = oldLabels
			labels, err := labelsForService(args, *oldLabels, args.newVersion, processName, args.newVersionSpec[processName])
			if err != nil {
				return nil, err
			}
			newLabelsMap[processName] = &labels
			totalUnits += labels.labels.AppReplicas()
		}
		err := args.app.SetQuotaInUse(totalUnits)
		if err != nil {
			return nil, err
		}
		for _, processName := range toDeployProcesses {
			labels := newLabelsMap[processName]
			ectx, cancel := args.event.CancelableContext(context.Background())
			err = args.manager.DeployService(ectx, args.app, processName, labels.labels, labels.realReplicas, args.newVersion, args.preserveVersions)
			cancel()
			if err != nil {
				break
			}
			deployedProcesses[processName] = oldLabelsMap[processName]
		}
		if err != nil {
			rollbackAddedProcesses(args, deployedProcesses)
			return nil, err
		}
		return deployedProcesses, nil
	},
	Backward: func(ctx action.BWContext) {
		args := ctx.Params[0].(*pipelineArgs)
		deployedProcesses := ctx.FWResult.(map[string]*labelReplicas)
		rollbackAddedProcesses(args, deployedProcesses)
	},
}

var updateImageInDB = &action.Action{
	Name: "update-image-in-db",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(*pipelineArgs)
		err := args.newVersion.CommitSuccessful()
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return ctx.Previous, nil
	},
}

var removeOldServices = &action.Action{
	Name: "remove-old-services",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(*pipelineArgs)
		err := removeOld(args)
		if err != nil {
			log.Errorf("ignored error removing old services for app %s: %+v", args.app.GetName(), err)
		}
		if !args.preserveVersions {
			err = args.manager.CleanupServices(args.app, args.newVersion)
			if err != nil {
				log.Errorf("ignored error cleaning up services for app %s: %+v", args.app.GetName(), err)
			}
		}
		return nil, nil
	},
}

func removeOld(args *pipelineArgs) error {
	if args.oldVersion == nil {
		return nil
	}
	oldProcs, err := args.oldVersion.Processes()
	if err != nil {
		return err
	}
	old := set.FromMap(oldProcs)
	new := set.FromMap(args.newVersionSpec)
	errs := tsuruErrors.NewMultiError()
	for processName := range old.Difference(new) {
		err = args.manager.RemoveService(args.app, processName, args.oldVersion)
		if err != nil {
			errs.Add(err)
		}
	}
	return errs.ToError()
}
