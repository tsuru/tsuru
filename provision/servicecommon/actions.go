// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/servicemanager"
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
	oldVersionNumber int
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
	RemoveService(ctx context.Context, a provision.App, processName string, versionNumber int) error
	CurrentLabels(ctx context.Context, a provision.App, processName string, versionNumber int) (*provision.LabelSet, *int32, error)
	DeployService(ctx context.Context, a provision.App, processName string, labels *provision.LabelSet, replicas int, version appTypes.AppVersion, preserveVersions bool) error
	CleanupServices(ctx context.Context, a provision.App, versionNumber int, preserveOldVersions bool) error
}

// RunServicePipeline runs a pipeline for deploy a service with multiple
// processes. oldVersion is an int instead of a AppVersion because it may not
// exist in our data store anymore.
func RunServicePipeline(ctx context.Context, manager ServiceManager, oldVersionNumber int, args provision.DeployArgs, updateSpec ProcessSpec) error {
	oldVersion, err := servicemanager.AppVersion.VersionByImageOrVersion(ctx, args.App, strconv.Itoa(oldVersionNumber))
	if err != nil {
		if !appTypes.IsInvalidVersionError(err) {
			return errors.WithStack(err)
		}
		log.Errorf("unable to find version %d for app %q: %v", oldVersionNumber, args.App.GetName(), err)
	}
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
	return pipeline.Execute(ctx, &pipelineArgs{
		manager:          manager,
		app:              args.App,
		preserveVersions: args.PreserveVersions,
		oldVersionNumber: oldVersionNumber,
		oldVersion:       oldVersion,
		newVersion:       args.Version,
		newVersionSpec:   newSpec,
		event:            args.Event,
	})
}

func rollbackAddedProcesses(ctx context.Context, args *pipelineArgs, processes map[string]*labelReplicas) error {
	errors := tsuruErrors.NewMultiError()
	for processName, oldLabels := range processes {
		if oldLabels.labels == nil {
			if err := args.manager.RemoveService(ctx, args.app, processName, args.newVersion.Version()); err != nil {
				errors.Add(fmt.Errorf("error removing service for %s[%s] [version %d]: %+v", args.app.GetName(), processName, args.newVersion.Version(), err))
			}
			continue
		}
		if args.oldVersion == nil {
			errors.Add(fmt.Errorf("unable to rollback service for %s[%s] to version %d, version not found anymore", args.app.GetName(), processName, args.oldVersionNumber))
			continue
		}
		err := args.manager.DeployService(context.Background(), args.app, processName, oldLabels.labels, oldLabels.realReplicas, args.oldVersion, args.preserveVersions)
		if err != nil {
			errors.Add(fmt.Errorf("error rolling back updated service for %s[%s] [version %d]: %+v", args.app.GetName(), processName, args.oldVersionNumber, err))
		}
	}
	return errors.ToError()
}

func rawLabelsAndReplicas(ctx context.Context, args *pipelineArgs, processName string, versionNumber int) (*labelReplicas, error) {
	if versionNumber == 0 {
		return &labelReplicas{}, nil
	}
	labels, replicas, err := args.manager.CurrentLabels(ctx, args.app, processName, versionNumber)
	if err != nil {
		return nil, err
	}
	if labels == nil {
		return &labelReplicas{}, nil
	}
	lr := &labelReplicas{labels: labels}
	if replicas != nil {
		lr.realReplicas = int(*replicas)
	}
	return lr, nil
}

func labelsForService(ctx context.Context, args *pipelineArgs, oldLabels labelReplicas, newVersion appTypes.AppVersion, processName string, pState ProcessState) (labelReplicas, error) {
	restartCount := 0
	isStopped := false
	isAsleep := false
	if oldLabels.labels != nil {
		restartCount = oldLabels.labels.Restarts()
		isStopped = oldLabels.labels.IsStopped()
		isAsleep = oldLabels.labels.IsAsleep()
	}
	if pState.Increment != 0 {
		oldLabels.realReplicas += pState.Increment
		if oldLabels.realReplicas < 0 {
			return oldLabels, errors.New("cannot have less than 0 units")
		}
	}
	if pState.Start || pState.Restart {
		if oldLabels.realReplicas == 0 {
			oldLabels.realReplicas = 1
		}
		isStopped = false
		isAsleep = false
	}
	labels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App:     args.app,
		Process: processName,
		Version: newVersion.Version(),
	})
	if err != nil {
		return oldLabels, err
	}
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
			oldLabels, err := rawLabelsAndReplicas(ctx.Context, args, processName, args.oldVersionNumber)
			if err != nil {
				return nil, err
			}
			oldLabelsMap[processName] = oldLabels
			labels, err := labelsForService(ctx.Context, args, *oldLabels, args.newVersion, processName, args.newVersionSpec[processName])
			if err != nil {
				return nil, err
			}
			newLabelsMap[processName] = &labels
			totalUnits += labels.realReplicas
		}
		var err error
		for _, processName := range toDeployProcesses {
			labels := newLabelsMap[processName]
			ectx, cancel := args.event.CancelableContext(ctx.Context)
			err = args.manager.DeployService(ectx, args.app, processName, labels.labels, labels.realReplicas, args.newVersion, args.preserveVersions)
			cancel()
			if err != nil {
				break
			}
			deployedProcesses[processName] = oldLabelsMap[processName]
		}
		errs := tsuruErrors.NewMultiError()
		if err != nil {
			errs.Add(err)
			if nerr := rollbackAddedProcesses(ctx.Context, args, deployedProcesses); nerr != nil {
				errs.Add(nerr)
			}
		}
		return deployedProcesses, errs.ToError()
	},
	Backward: func(ctx action.BWContext) {
		args := ctx.Params[0].(*pipelineArgs)
		deployedProcesses := ctx.FWResult.(map[string]*labelReplicas)
		rollbackAddedProcesses(ctx.Context, args, deployedProcesses)
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
		err := removeOld(ctx.Context, args)
		if err != nil {
			log.Errorf("ignored error removing old services for app %s: %+v", args.app.GetName(), err)
		}
		err = args.manager.CleanupServices(ctx.Context, args.app, args.newVersion.Version(), args.preserveVersions)
		if err != nil {
			log.Errorf("ignored error cleaning up services for app %s: %+v", args.app.GetName(), err)
		}
		return nil, nil
	},
}

func removeOld(ctx context.Context, args *pipelineArgs) error {
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
		err = args.manager.RemoveService(ctx, args.app, processName, args.oldVersion.Version())
		if err != nil {
			errs.Add(err)
		}
	}
	return errs.ToError()
}
