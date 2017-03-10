// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"sort"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/set"
)

type ProcessState struct {
	Stop      bool
	Start     bool
	Restart   bool
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
}

type ServiceManager interface {
	DeployService(a provision.App, processName string, count ProcessState, image string) error
	RemoveService(a provision.App, processName string) error
}

func RunServicePipeline(manager ServiceManager, a provision.App, newImg string, updateSpec ProcessSpec) error {
	curImg, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return err
	}
	currentImageData, err := image.GetImageCustomData(curImg)
	if err != nil {
		return err
	}
	currentSpec := ProcessSpec{}
	for p := range currentImageData.Processes {
		currentSpec[p] = ProcessState{}
	}
	newImageData, err := image.GetImageCustomData(newImg)
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
	})
}

func rollbackAddedProcesses(args *pipelineArgs, processes []string) {
	for _, processName := range processes {
		var err error
		if count, in := args.currentImageSpec[processName]; in {
			err = args.manager.DeployService(args.app, processName, count, args.currentImage)
		} else {
			err = args.manager.RemoveService(args.app, processName)
		}
		if err != nil {
			log.Errorf("error rolling back updated service for %s[%s]: %+v", args.app.GetName(), processName, err)
		}
	}
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
		for _, processName := range toDeployProcesses {
			err = args.manager.DeployService(args.app, processName, args.newImageSpec[processName], args.newImage)
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
