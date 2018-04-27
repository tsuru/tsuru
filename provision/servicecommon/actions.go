// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/set"
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
	newImage         string
	newImageSpec     ProcessSpec
	currentImage     string
	currentImageSpec ProcessSpec
	event            *event.Event
}

type labelReplicas struct {
	labels       *provision.LabelSet
	realReplicas int
}

type ServiceManager interface {
	RemoveService(a provision.App, processName string) error
	CurrentLabels(a provision.App, processName string) (*provision.LabelSet, error)
	DeployService(ctx context.Context, a provision.App, processName string, labels *provision.LabelSet, replicas int, image string) error
}

func RunServicePipeline(manager ServiceManager, a provision.App, newImg string, updateSpec ProcessSpec, evt *event.Event) error {
	curImg, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return err
	}
	currentImageData, err := image.GetImageMetaData(curImg)
	if err != nil {
		return err
	}
	currentSpec := ProcessSpec{}
	for p := range currentImageData.Processes {
		currentSpec[p] = ProcessState{}
	}
	newImageData, err := image.GetImageMetaData(newImg)
	if err != nil {
		return err
	}
	if len(newImageData.Processes) == 0 {
		return errors.Errorf("no process information found deploying image %q", newImg)
	}
	newSpec := ProcessSpec{}
	for p := range newImageData.Processes {
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
		app:              a,
		newImage:         newImg,
		newImageSpec:     newSpec,
		currentImage:     curImg,
		currentImageSpec: currentSpec,
		event:            evt,
	})
}

func rollbackAddedProcesses(args *pipelineArgs, processes []string) {
	for _, processName := range processes {
		var err error
		if state, in := args.currentImageSpec[processName]; in {
			var labels *labelReplicas
			labels, err = labelsForService(args, processName, state)
			if err == nil {
				err = args.manager.DeployService(context.Background(), args.app, processName, labels.labels, labels.realReplicas, args.currentImage)
			}
		} else {
			err = args.manager.RemoveService(args.app, processName)
		}
		if err != nil {
			log.Errorf("error rolling back updated service for %s[%s]: %+v", args.app.GetName(), processName, err)
		}
	}
}

func labelsForService(args *pipelineArgs, processName string, pState ProcessState) (*labelReplicas, error) {
	oldLabels, err := args.manager.CurrentLabels(args.app, processName)
	if err != nil {
		return nil, err
	}
	replicas := 0
	restartCount := 0
	isStopped := false
	isAsleep := false
	if oldLabels != nil {
		replicas = oldLabels.AppReplicas()
		restartCount = oldLabels.Restarts()
		isStopped = oldLabels.IsStopped()
		isAsleep = oldLabels.IsAsleep()
	}
	if pState.Increment != 0 {
		replicas += pState.Increment
		if replicas < 0 {
			return nil, errors.New("cannot have less than 0 units")
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
	})
	if err != nil {
		return nil, err
	}
	realReplicas := replicas
	if isStopped || pState.Stop {
		realReplicas = 0
		labels.SetStopped()
	}
	if isAsleep || pState.Sleep {
		labels.SetAsleep()
	}
	if pState.Restart {
		restartCount++
		labels.SetRestarts(restartCount)
	}
	return &labelReplicas{labels: labels, realReplicas: realReplicas}, nil
}

var updateServices = &action.Action{
	Name: "update-services",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(*pipelineArgs)
		var (
			toDeployProcesses []string
			deployedProcesses []string
			err               error
		)
		for processName := range args.newImageSpec {
			toDeployProcesses = append(toDeployProcesses, processName)
		}
		sort.Strings(toDeployProcesses)
		totalUnits := 0
		labelsMap := map[string]*labelReplicas{}
		for _, processName := range toDeployProcesses {
			var labels *labelReplicas
			labels, err = labelsForService(args, processName, args.newImageSpec[processName])
			if err != nil {
				return nil, err
			}
			labelsMap[processName] = labels
			totalUnits += labels.labels.AppReplicas()
		}
		err = args.app.SetQuotaInUse(totalUnits)
		if err != nil {
			return nil, err
		}
		for _, processName := range toDeployProcesses {
			labels := labelsMap[processName]
			ectx, cancel := args.event.CancelableContext(context.Background())
			err = args.manager.DeployService(ectx, args.app, processName, labels.labels, labels.realReplicas, args.newImage)
			cancel()
			if err != nil {
				break
			}
			deployedProcesses = append(deployedProcesses, processName)
		}
		if err != nil {
			rollbackAddedProcesses(args, deployedProcesses)
			return nil, err
		}
		return deployedProcesses, nil
	},
	Backward: func(ctx action.BWContext) {
		args := ctx.Params[0].(*pipelineArgs)
		deployedProcesses := ctx.FWResult.([]string)
		rollbackAddedProcesses(args, deployedProcesses)
	},
}

var updateImageInDB = &action.Action{
	Name: "update-image-in-db",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(*pipelineArgs)
		err := image.AppendAppImageName(args.app.GetName(), args.newImage)
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
		old := set.FromMap(args.currentImageSpec)
		new := set.FromMap(args.newImageSpec)
		for processName := range old.Difference(new) {
			err := args.manager.RemoveService(args.app, processName)
			if err != nil {
				log.Errorf("ignored error removing unwanted service for %s[%s]: %+v", args.app.GetName(), processName, err)
			}
		}
		return nil, nil
	},
}
