// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/dotcloud/docker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
)

var createContainer = action.Action{
	Name: "create-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(provision.App)
		cont := container{
			AppName: app.GetName(),
			Type:    app.GetPlatform(),
		}
		port, err := getPort()
		if err != nil {
			return nil, err
		}
		user, err := config.GetString("docker:ssh:user")
		if err != nil {
			return nil, err
		}
		imageId := ctx.Params[1].(string)
		cmds := ctx.Params[2].([]string)
		config := docker.Config{
			Image:        imageId,
			Cmd:          cmds,
			PortSpecs:    []string{port},
			User:         user,
			AttachStdin:  false,
			AttachStdout: false,
			AttachStderr: false,
		}
		_, c, err := dockerCluster.CreateContainer(&config)
		if err != nil {
			return nil, err
		}
		cont.ID = c.ID
		cont.Port = port
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(container)
		dockerCluster.RemoveContainer(c.ID)
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
			log.Print(err)
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
		return nil, err
	},
	Backward: func(ctx action.BWContext) {
	},
}
