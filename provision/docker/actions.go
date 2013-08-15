// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
)

var createContainer = action.Action{
	Name: "create-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(provision.App)
		imageId := ctx.Params[1].(string)
		cmds := ctx.Params[2].([]string)
		log.Printf("create container for app %s, based on image %s, with cmds %s", app.GetName(), imageId, cmds)
		cont, err := newContainer(app, imageId, cmds)
		if err != nil {
			log.Printf("error on create container for app %s - %s", app.GetName(), err.Error())
			return nil, err
		}
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(container)
		dockerCluster().RemoveContainer(c.ID)
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

var setIp = action.Action{
	Name: "set-ip",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		c := ctx.Previous.(container)
		ip, err := c.ip()
		if err != nil {
			return nil, err
		}
		c.IP = ip
		return c, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}

var setHostPort = action.Action{
	Name: "set-host-port",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		c := ctx.Previous.(container)
		hostPort, err := c.hostPort()
		if err != nil {
			return nil, err
		}
		c.HostPort = hostPort
		return c, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}

var insertContainer = action.Action{
	Name: "insert-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		c := ctx.Previous.(container)
		c.Status = "created"
		coll := collection()
		defer coll.Database.Session.Close()
		if err := coll.Insert(c); err != nil {
			log.Printf("error on inserting container into database %s - %s", c.ID, err.Error())
			return nil, err
		}
		return c, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(container)
		coll := collection()
		defer coll.Database.Session.Close()
		coll.RemoveId(c.ID)
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
		log.Printf("starting container %s", c.ID)
		err := dockerCluster().StartContainer(c.ID)
		if err != nil {
			log.Printf("error on start container %s - %s", c.ID, err)
			return nil, err
		}
		return c, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}
