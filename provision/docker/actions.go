// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/queue"
	"labix.org/v2/mgo/bson"
	"net"
	"strings"
)

var insertEmptyContainerInDB = action.Action{
	Name: "insert-empty-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(provision.App)
		contName := containerName()
		cont := container{
			AppName: app.GetName(),
			Type:    app.GetPlatform(),
			Name:    contName,
			Status:  "created",
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
		app, ok := ctx.Params[0].(provision.App)
		if !ok {
			return nil, errors.New("First parameter must be a provision.App.")
		}
		go injectEnvsAndRestart(app)
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}

var saveUnits = action.Action{
	Name: "save-units",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		a, ok := ctx.Params[0].(*app.App)
		if !ok {
			return nil, errors.New("First parameter must be a *app.App.")
		}
		a, err := app.GetByName(a.Name)
		if err != nil {
			return nil, err
		}
		containers, err := listAppContainers(a.GetName())
		if err != nil {
			return nil, err
		}
		for _, c := range containers {
			var status string
			addr := strings.Replace(c.getAddress(), "http://", "", 1)
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				status = provision.StatusUnreachable.String()
			} else {
				conn.Close()
				status = provision.StatusStarted.String()
			}
			u := app.Unit{
				Name:  c.ID,
				Type:  c.Type,
				Ip:    c.HostAddr,
				State: status,
			}
			a.AddUnit(&u)
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		conn.Apps().Update(bson.M{"name": a.Name}, a)
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}

var bindService = action.Action{
	Name: "bind-service",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		a, ok := ctx.Params[0].(provision.App)
		if !ok {
			return nil, errors.New("First parameter must be a provision.App.")
		}
		for _, u := range a.ProvisionedUnits() {
			msg := queue.Message{
				Action: app.BindService,
				Args:   []string{a.GetName(), u.GetName()},
			}
			go app.Enqueue(msg)
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}

var provisionAddUnitsToHost = action.Action{
	Name: "provision-add-units-to-host",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		a := ctx.Params[0].(provision.App)
		n := ctx.Params[1].(int)
		destinationHost := ctx.Params[2].(string)
		units, err := addUnitsWithHost(a, uint(n), destinationHost)
		if err != nil {
			return nil, err
		}
		return units, nil
	},
	Backward: func(ctx action.BWContext) {
		a := ctx.Params[0].(provision.App)
		units := ctx.FWResult.([]provision.Unit)
		var provisioner dockerProvisioner
		for _, unit := range units {
			provisioner.RemoveUnit(a, unit.Name)
		}
	},
	MinParams: 3,
}
