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
