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
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type ProcessState struct {
	Stop    bool
	Start   bool
	Restart bool

	// SetReplicas is used to set the number of replicas for the process, have precedence over Increment attribute.
	SetReplicas int

	// Increment is used to increment the number of replicas for the process, when Increment is set SetReplicas may not set.
	Increment int
}

type ProcessSpec map[string]ProcessState

type pipelineArgs struct {
	manager          ServiceManager
	app              *appTypes.App
	oldVersionNumber int
	oldVersion       appTypes.AppVersion
	newVersion       appTypes.AppVersion
	newVersionSpec   ProcessSpec
	event            *event.Event
	preserveVersions bool
	overrideVersions bool
}

type labelReplicas struct {
	labels       *provision.LabelSet
	realReplicas int
}

type ServiceManager interface {
	RemoveService(ctx context.Context, a *appTypes.App, processName string, versionNumber int) error
	CurrentLabels(ctx context.Context, a *appTypes.App, processName string, versionNumber int) (*provision.LabelSet, *int32, error)
	DeployService(ctx context.Context, o DeployServiceOpts) error
	CleanupServices(ctx context.Context, a *appTypes.App, versionNumber int, preserveOldVersions bool) error
}

type DeployServiceOpts struct {
	App              *appTypes.App
	ProcessName      string
	Labels           *provision.LabelSet
	Replicas         int
	Version          appTypes.AppVersion
	PreserveVersions bool
	OverrideVersions bool
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
		log.Errorf("unable to find version %d for app %q: %v", oldVersionNumber, args.App.Name, err)
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
		overrideVersions: args.OverrideVersions,
	})
}

func rollbackAddedProcesses(ctx context.Context, args *pipelineArgs, processes map[string]*labelReplicas) error {
	errors := tsuruErrors.NewMultiError()
	for processName, oldLabels := range processes {
		if oldLabels.labels == nil {
			if err := args.manager.RemoveService(ctx, args.app, processName, args.newVersion.Version()); err != nil {
				errors.Add(fmt.Errorf("error removing service for %s[%s] [version %d]: %+v", args.app.Name, processName, args.newVersion.Version(), err))
			}
			continue
		}
		if args.oldVersion == nil {
			errors.Add(fmt.Errorf("unable to rollback service for %s[%s] to version %d, version not found anymore", args.app.Name, processName, args.oldVersionNumber))
			continue
		}
		err := args.manager.DeployService(context.Background(), DeployServiceOpts{
			App:              args.app,
			ProcessName:      processName,
			Labels:           oldLabels.labels,
			Replicas:         oldLabels.realReplicas,
			Version:          args.oldVersion,
			PreserveVersions: args.preserveVersions,
		})
		if err != nil {
			errors.Add(fmt.Errorf("error rolling back updated service for %s[%s] [version %d]: %+v", args.app.Name, processName, args.oldVersionNumber, err))
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
	if oldLabels.labels != nil {
		restartCount = oldLabels.labels.Restarts()
		isStopped = oldLabels.labels.IsStopped()
	}

	if pState.SetReplicas != 0 {
		oldLabels.realReplicas = pState.SetReplicas
	} else if pState.Increment != 0 {
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
	if pState.Restart {
		restartCount++
		labels.SetRestarts(restartCount)
	}
	appMetadata := provision.GetAppMetadata(args.app, processName)
	for _, l := range appMetadata.Labels {
		labels.RawLabels[l.Name] = l.Value
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
			err = args.manager.DeployService(ctx.Context, DeployServiceOpts{
				App:              args.app,
				ProcessName:      processName,
				Labels:           labels.labels,
				Replicas:         labels.realReplicas,
				Version:          args.newVersion,
				PreserveVersions: args.preserveVersions,
				OverrideVersions: args.overrideVersions,
			})
			if err != nil {
				break
			}
			deployedProcesses[processName] = oldLabelsMap[processName]
		}
		errs := tsuruErrors.NewMultiError()
		if err != nil {
			errs.Add(err)
			rollbackCtx := tsuruNet.WithoutCancel(ctx.Context)
			if nerr := rollbackAddedProcesses(rollbackCtx, args, deployedProcesses); nerr != nil {
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
			log.Errorf("ignored error removing old services for app %s: %+v", args.app.Name, err)
		}
		err = args.manager.CleanupServices(ctx.Context, args.app, args.newVersion.Version(), args.preserveVersions)
		if err != nil {
			log.Errorf("ignored error cleaning up services for app %s: %+v", args.app.Name, err)
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
	removedProcesses := old.Difference(new)

	// Cleanup autoscales for removed processes
	prov, err := pool.GetProvisionerForPool(ctx, args.app.Pool)
	if err != nil {
		log.Errorf("unable to get provisioner for pool %s while cleaning up autoscales: %v", args.app.Pool, err)
	} else {
		autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
		if ok {
			for processName := range removedProcesses {
				err := autoscaleProv.RemoveAutoScale(ctx, args.app, processName)
				if err != nil {
					log.Errorf("error removing autoscale for process %s[%s]: %v", args.app.Name, processName, err)
				}
			}
		}
	}

	errs := tsuruErrors.NewMultiError()
	for processName := range removedProcesses {
		err = args.manager.RemoveService(ctx, args.app, processName, args.oldVersion.Version())
		if err != nil {
			errs.Add(err)
		}
	}
	return errs.ToError()
}
