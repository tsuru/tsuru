// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/repository"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"strings"
)

// deployCmds returns the commands that is used when provisioner
// deploy an unit.
func deployCmds(app provision.App, version string) ([]string, error) {
	deployCmd, err := config.GetString("docker:deploy-cmd")
	if err != nil {
		return nil, err
	}
	appRepo := repository.ReadOnlyURL(app.GetName())
	cmds := []string{deployCmd, appRepo}
	return cmds, nil
}

// runCmds returns the commands that should be passed when the
// provisioner will run an unit.
func runCmds(imageId string) ([]string, error) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return nil, err
	}
	runCmd, err := config.GetString("docker:run-cmd:bin")
	if err != nil {
		return nil, err
	}
	ssh, err := sshCmds()
	if err != nil {
		return nil, err
	}
	sshCmd := strings.Join(ssh, " && ")
	cmd := fmt.Sprintf("%s && %s", runCmd, sshCmd)
	cmds := []string{docker, "run", "-d", "-t", "/bin/bash", "-c", cmd}
	return cmds, nil
}

// sshCmds returns the commands needed to start a ssh daemon.
func sshCmds() ([]string, error) {
	addKeyCommand, err := config.GetString("docker:ssh:add-key-cmd")
	if err != nil {
		return nil, err
	}
	keyFile, err := config.GetString("docker:ssh:public-key")
	if err != nil {
		if u, err := user.Current(); err == nil {
			keyFile = path.Join(u.HomeDir, ".ssh", "id_rsa.pub")
		} else {
			keyFile = os.ExpandEnv("${HOME}/.ssh/id_rsa.pub")
		}
	}
	f, err := filesystem().Open(keyFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	keyContent, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	sshdPath, err := config.GetString("docker:ssh:sshd-path")
	if err != nil {
		sshdPath = "/usr/sbin/sshd"
	}
	return []string{
		fmt.Sprintf("%s %s", addKeyCommand, bytes.TrimSpace(keyContent)),
		sshdPath + " -D",
	}, nil
}
