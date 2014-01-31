// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"github.com/fsouza/go-dockerclient"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"labix.org/v2/mgo/bson"
	"net"
	"strings"
)

var createContainer = action.Action{
	Name: "create-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(provision.App)
		imageId := ctx.Params[1].(string)
		cmds := ctx.Params[2].([]string)
		log.Debugf("create container for app %s, based on image %s, with cmds %s", app.GetName(), imageId, cmds)
		cont, err := newContainer(app, imageId, cmds)
		if err != nil {
			log.Errorf("error on create container for app %s - %s", app.GetName(), err)
			return nil, err
		}
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(container)
		dockerCluster().RemoveContainer(docker.RemoveContainerOptions{ID: c.ID})
		coll := collection()
		defer coll.Close()
		coll.Remove(bson.M{"id": c.ID})
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
		a, err := app.GetAppByName(a.Name)
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
