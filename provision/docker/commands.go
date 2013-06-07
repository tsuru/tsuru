// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/provision"
)

func deployCmds(app provision.App) ([]string, error) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return nil, err
	}
	deployCmd, err := config.GetString("docker:deploy-cmd")
	if err != nil {
		return nil, err
	}
	imageName := getImage(app)
	cmds := []string{docker, "run", imageName, deployCmd}
	return cmds, nil
}

// runCmds returns the commands that should be passed when the
// provisioner will run an unit.
func runCmds(app provision.App) ([]string, error) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return nil, err
	}
	runCmd, err := config.GetString("docker:run-cmd:bin")
	if err != nil {
		return nil, err
	}
	port, err := config.GetString("docker:run-cmd:port")
	if err != nil {
		return nil, err
	}
	imageName := getImage(app)
	cmds := []string{docker, "run", "-d", "-t", "-p", port, imageName, "/bin/bash", "-c", runCmd}
	return cmds, nil
}
