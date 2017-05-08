// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"io"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
)

var ErrDeployCanceled = errors.New("deploy canceled by user action")

type runContainerActionsArgs struct {
	app              provision.App
	processName      string
	imageID          string
	commands         []string
	destinationHosts []string
	writer           io.Writer
	isDeploy         bool
	buildingImage    string
	builder          *dockerBuilder
	client           *docker.Client
	exposedPort      string
	event            *event.Event
}

type containersToAdd struct {
	Quantity int
	Status   provision.Status
}

type callbackFunc func(*Container, chan *Container) error

type rollbackFunc func(*Container)

func checkCanceled(evt *event.Event) error {
	if evt == nil {
		return nil
	}
	canceled, err := evt.AckCancel()
	if err != nil {
		log.Errorf("unable to check if event should be canceled, ignoring: %s", err)
		return nil
	}
	if canceled {
		return ErrDeployCanceled
	}
	return nil
}

var createContainer = action.Action{
	Name: "create-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(runContainerActionsArgs)
		if err := checkCanceled(args.event); err != nil {
			return nil, err
		}
		initialStatus := provision.StatusBuilding
		contName := args.app.GetName() + "-" + randomString()
		cont := Container{
			AppName:       args.app.GetName(),
			ProcessName:   args.processName,
			Type:          args.app.GetPlatform(),
			Name:          contName,
			Status:        initialStatus.String(),
			Image:         args.imageID,
			BuildingImage: args.buildingImage,
			ExposedPort:   args.exposedPort,
		}
		log.Debugf("create container for app %s, based on image %s, with cmds %s", args.app.GetName(), args.imageID, args.commands)
		err := cont.Create(&CreateContainerArgs{
			ImageID:          args.imageID,
			Commands:         args.commands,
			App:              args.app,
			Deploy:           args.isDeploy,
			Client:           args.client,
			DestinationHosts: args.destinationHosts,
			ProcessName:      args.processName,
			Building:         true,
		})
		if err != nil {
			log.Errorf("error on create container for app %s - %s", args.app.GetName(), err)
			return nil, err
		}
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(Container)
		args := ctx.Params[0].(runContainerActionsArgs)
		err := args.client.RemoveContainer(docker.RemoveContainerOptions{ID: c.ID})
		if err != nil {
			log.Errorf("Failed to remove the container %q: %s", c.ID, err)
		}
	},
}

var commitContainer = action.Action{
	Name: "commit-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(runContainerActionsArgs)
		if err := checkCanceled(args.event); err != nil {
			return nil, err
		}
		c, ok := ctx.Previous.(Container)
		if !ok {
			return nil, errors.New("previous result must be a container")
		}
		fmt.Fprintf(args.writer, "\n---- Building application image ----\n")
		imageID, err := c.Commit(args.client, args.writer)
		if err != nil {
			log.Errorf("error on commit container %s - %s", c.ID, err)
			return nil, err
		}
		fmt.Fprintf(args.writer, " ---> Cleaning up\n")
		c.Remove(args.client)
		return imageID, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}
