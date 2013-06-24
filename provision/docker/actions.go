// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/dotcloud/docker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/action"
)

var createContainer = action.Action{
	Name: "create-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		port, err := getPort()
		if err != nil {
			return nil, err
		}
		user, err := config.GetString("docker:ssh:user")
		if err != nil {
			return nil, err
		}
		imageId := ctx.Params[0].(string)
		cmds := ctx.Params[1].([]string)
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
		cont := container{ID: c.ID}
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}
