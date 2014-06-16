// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/queue"
	"io"
	"labix.org/v2/mgo/bson"
)

var insertEmptyContainerInDB = action.Action{
	Name: "insert-empty-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(provision.App)
		imageId := ctx.Params[1].(string)
		contName := containerName()
		cont := container{
			AppName: app.GetName(),
			Type:    app.GetPlatform(),
			Name:    contName,
			Status:  "created",
			Image:   imageId,
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
		app := ctx.Params[0].(provision.App)
		imageId := ctx.Params[1].(string)
		cmds := ctx.Params[2].([]string)
		var destinationHosts []string
		if len(ctx.Params) > 3 {
			destinationHosts = ctx.Params[3].([]string)
		}
		log.Debugf("create container for app %s, based on image %s, with cmds %s", app.GetName(), imageId, cmds)
		err := cont.create(app, imageId, cmds, destinationHosts...)
		if err != nil {
			log.Errorf("error on create container for app %s - %s", app.GetName(), err)
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
		ip, hostPort, err := c.networkInfo()
		if err != nil {
			return nil, err
		}
		c.IP = ip
		c.HostPort = hostPort
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

var injectEnvirons = action.Action{
	Name: "inject-environs",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		opts, ok := ctx.Params[0].(app.DeployOptions)
		if !ok {
			return nil, errors.New("First parameter must be DeployOptions")
		}
		go injectEnvsAndRestart(opts.App)
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
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
		a := ctx.Params[0].(provision.App)
		unit := ctx.FWResult.(provision.Unit)
		var provisioner dockerProvisioner
		err := provisioner.RemoveUnit(a, unit.Name)
		if err != nil {
			log.Errorf("Error removing added unit %s - %s", unit.Name, err)
		}
	},
	MinParams: 2,
}

var provisionRemoveOldUnit = action.Action{
	Name: "provision-remove-old-unit",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		unit := ctx.Previous.(provision.Unit)
		a := ctx.Params[0].(provision.App)
		cont := ctx.Params[2].(container)
		var provisioner dockerProvisioner
		err := provisioner.RemoveUnit(a, cont.ID)
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
		w, ok := ctx.Params[4].(io.Writer)
		if !ok {
			return nil, errors.New("Fifth parameter must be a io.Writer.")
		}
		err := c.logs(w)
		if err != nil {
			log.Errorf("error on get logs for container %s - %s", c.ID, err)
			return nil, err
		}
		_, err = dockerCluster().WaitContainer(c.ID)
		if err != nil {
			log.Errorf("Process failed for container %q: %s", c.ID, err)
			return nil, err
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
	MinParams: 5,
}
