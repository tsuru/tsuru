// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/queue"
	"io"
	"labix.org/v2/mgo/bson"
)

type runContainerActionsArgs struct {
	app              provision.App
	imageID          string
	commands         []string
	destinationHosts []string
	privateKey       []byte
	writer           io.Writer
}

var insertEmptyContainerInDB = action.Action{
	Name: "insert-empty-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(runContainerActionsArgs)
		contName := containerName()
		cont := container{
			AppName:    args.app.GetName(),
			Type:       args.app.GetPlatform(),
			Name:       contName,
			Status:     "created",
			Image:      args.imageID,
			PrivateKey: string(args.privateKey),
		}
		coll := collection()
		defer coll.Close()
		if err := coll.Insert(cont); err != nil {
			log.Errorf("error on inserting container into database %s - %s", cont.Name, err)
			return nil, err
		}
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(container)
		coll := collection()
		defer coll.Close()
		coll.Remove(bson.M{"name": c.Name})
	},
}

var updateContainerInDB = action.Action{
	Name: "update-database-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		coll := collection()
		defer coll.Close()
		cont := ctx.Previous.(container)
		err := coll.Update(bson.M{"name": cont.Name}, cont)
		if err != nil {
			log.Errorf("error on updating container into database %s - %s", cont.ID, err)
			return nil, err
		}
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}

var createContainer = action.Action{
	Name: "create-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		cont := ctx.Previous.(container)
		args := ctx.Params[0].(runContainerActionsArgs)
		log.Debugf("create container for app %s, based on image %s, with cmds %s", args.app.GetName(), args.imageID, args.commands)
		err := cont.create(args.app, args.imageID, args.commands, args.destinationHosts...)
		if err != nil {
			log.Errorf("error on create container for app %s - %s", args.app.GetName(), err)
			return nil, err
		}
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(container)
		err := dockerCluster().RemoveContainer(docker.RemoveContainerOptions{ID: c.ID})
		if err != nil {
			log.Errorf("Failed to remove the container %q: %s", c.ID, err)
		}
	},
}

var setNetworkInfo = action.Action{
	Name: "set-network-info",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		c := ctx.Previous.(container)
		info, err := c.networkInfo()
		if err != nil {
			return nil, err
		}
		c.IP = info.IP
		c.HostPort = info.HTTPHostPort
		c.SSHHostPort = info.SSHHostPort
		return c, nil
	},
}

var addRoute = action.Action{
	Name: "add-route",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		c := ctx.Previous.(container)
		r, err := getRouter()
		if err != nil {
			return nil, err
		}
		err = r.AddRoute(c.AppName, c.getAddress())
		return c, err
	},
	Backward: func(ctx action.BWContext) {
	},
}

var startContainer = action.Action{
	Name: "start-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		c := ctx.Previous.(container)
		log.Debugf("starting container %s", c.ID)
		err := c.start()
		if err != nil {
			log.Errorf("error on start container %s - %s", c.ID, err)
			return nil, err
		}
		return c, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(container)
		err := dockerCluster().StopContainer(c.ID, 10)
		if err != nil {
			log.Errorf("Failed to stop the container %q: %s", c.ID, err)
		}
	},
}

var bindService = action.Action{
	Name: "bind-service",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		opts, ok := ctx.Params[0].(app.DeployOptions)
		if !ok {
			return nil, errors.New("First parameter must be DeployOptions")
		}
		for _, u := range opts.App.Units() {
			msg := queue.Message{
				Action: app.BindService,
				Args:   []string{opts.App.GetName(), u.Name},
			}
			go app.Enqueue(msg)
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}

var provisionAddUnitToHost = action.Action{
	Name: "provision-add-unit-to-host",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		a := ctx.Params[0].(provision.App)
		destinationHost := ctx.Params[1].(string)
		units, err := addUnitsWithHost(a, 1, destinationHost)
		if err != nil {
			return nil, err
		}
		return units[0], nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.Params[2].(container)
		err := removeContainer(&c)
		if err != nil {
			log.Errorf("Error removing added unit %s - %s", c.ID, err)
		}
	},
	MinParams: 2,
}

var provisionRemoveOldUnit = action.Action{
	Name: "provision-remove-old-unit",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		unit := ctx.Previous.(provision.Unit)
		cont := ctx.Params[2].(container)
		err := removeContainer(&cont)
		if err != nil {
			return unit, err
		}
		return unit, nil
	},
	Backward: func(ctx action.BWContext) {
	},
	MinParams: 3,
}

var followLogsAndCommit = action.Action{
	Name: "follow-logs-and-commit",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		c, ok := ctx.Previous.(container)
		if !ok {
			return nil, errors.New("Previous result must be a container.")
		}
		args := ctx.Params[0].(runContainerActionsArgs)
		err := c.logs(args.writer)
		if err != nil {
			log.Errorf("error on get logs for container %s - %s", c.ID, err)
			return nil, err
		}
		status, err := dockerCluster().WaitContainer(c.ID)
		if err != nil {
			log.Errorf("Process failed for container %q: %s", c.ID, err)
			return nil, err
		}
		if status != 0 {
			return nil, fmt.Errorf("Exit status %d", status)
		}
		imageId, err := c.commit()
		if err != nil {
			log.Errorf("error on commit container %s - %s", c.ID, err)
			return nil, err
		}
		c.remove()
		return imageId, nil
	},
	Backward: func(ctx action.BWContext) {
	},
	MinParams: 1,
}
