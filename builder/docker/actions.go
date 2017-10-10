// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"io"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
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
	provisioner      provision.BuilderDeploy
	client           provision.BuilderDockerClient
	exposedPort      string
	event            *event.Event
	tarFile          io.Reader
}

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
		contName := args.app.GetName() + "-" + randomString()
		cont := Container{
			AppName:       args.app.GetName(),
			ProcessName:   args.processName,
			Type:          args.app.GetPlatform(),
			Name:          contName,
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

var uploadToContainer = action.Action{
	Name: "upload-to-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(runContainerActionsArgs)
		if err := checkCanceled(args.event); err != nil {
			return nil, err
		}
		c := ctx.Previous.(Container)
		log.Debugf("uploading tarfile to container %s", c.ID)
		uploadOpts := docker.UploadToContainerOptions{
			InputStream: args.tarFile,
			Path:        archiveDirPath,
		}
		err := args.client.UploadToContainer(c.ID, uploadOpts)
		if err != nil {
			log.Errorf("error on upload tarfile to container %s - %s", c.ID, err)
			return nil, err
		}
		return c, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}

var startContainer = action.Action{
	Name: "start-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(runContainerActionsArgs)
		if err := checkCanceled(args.event); err != nil {
			return nil, err
		}
		c := ctx.Previous.(Container)
		log.Debugf("starting container %s", c.ID)
		err := c.Start(&StartArgs{
			Client: args.client,
		})
		if err != nil {
			log.Errorf("error on start container %s - %s", c.ID, err)
			return nil, err
		}
		return c, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(Container)
		args := ctx.Params[0].(runContainerActionsArgs)
		err := args.client.StopContainer(c.ID, 10)
		if err != nil {
			log.Errorf("Failed to stop the container %q: %s", c.ID, err)
		}
	},
}

var followLogsAndCommit = action.Action{
	Name: "follow-logs-and-commit",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(runContainerActionsArgs)
		if err := checkCanceled(args.event); err != nil {
			return nil, err
		}
		c, ok := ctx.Previous.(Container)
		if !ok {
			return nil, errors.New("Previous result must be a container.")
		}
		type logsResult struct {
			status int
			err    error
		}
		doneCh := make(chan bool)
		canceledCh := make(chan error)
		resultCh := make(chan logsResult)
		go func() {
			for {
				err := checkCanceled(args.event)
				if err != nil {
					select {
					case <-doneCh:
					case canceledCh <- err:
					}
					return
				}
				select {
				case <-doneCh:
					return
				case <-time.After(time.Second):
				}
			}
		}()
		go func() {
			status, err := c.Logs(args.client, args.writer)
			select {
			case resultCh <- logsResult{status: status, err: err}:
			default:
			}
		}()
		select {
		case err := <-canceledCh:
			return nil, err
		case result := <-resultCh:
			doneCh <- true
			if result.err != nil {
				log.Errorf("error on get logs for container %s - %s", c.ID, result.err)
				return nil, result.err
			}
			if result.status != 0 {
				return nil, errors.Errorf("Exit status %d", result.status)
			}
		}
		if err := checkCanceled(args.event); err != nil {
			return nil, err
		}
		fmt.Fprintf(args.writer, "\n---- Building image ----\n")
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
	MinParams: 1,
}

var updateAppBuilderImage = action.Action{
	Name: "update-app-builder-image",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(runContainerActionsArgs)
		if err := checkCanceled(args.event); err != nil {
			return nil, err
		}
		err := image.AppendAppBuilderImageName(args.app.GetName(), args.buildingImage)
		if err != nil {
			return nil, errors.Wrap(err, "unable to save image name")
		}
		imgHistorySize := image.ImageHistorySize()
		allImages, err := image.ListAppBuilderImages(args.app.GetName())
		if err != nil {
			log.Errorf("Couldn't list images for cleaning: %s", err)
			return ctx.Previous, nil
		}
		limit := len(allImages) - imgHistorySize
		if limit > 0 {
			for _, imgName := range allImages[:limit] {
				args.provisioner.CleanImage(args.app.GetName(), imgName)
			}
		}
		return ctx.Previous, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}
