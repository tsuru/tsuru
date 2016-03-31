// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"fmt"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision/docker/container"
)

type DockerProvisioner interface {
	Cluster() *cluster.Cluster
	RegistryAuthConfig() docker.AuthConfiguration
}

const (
	BsDefaultName      = "big-sibling"
	bsDefaultImageName = "tsuru/bs:v1"
	bsHostProc         = "/prochost"
)

func InitializeBS() (bool, error) {
	bsNodeContainer, err := LoadNodeContainer("", BsDefaultName)
	if err != nil {
		return false, err
	}
	if len(bsNodeContainer.Config.Env) > 0 {
		return false, nil
	}
	tokenData, err := app.AuthScheme.AppLogin(app.InternalAppName)
	if err != nil {
		return false, err
	}
	token := tokenData.GetValue()
	conf := configFor(BsDefaultName)
	isSet, err := conf.SetFieldAtomic("", "Config.Env", []string{
		"TSURU_TOKEN=" + token,
	})
	if !isSet {
		// Already set by someone else, just bail out.
		app.AuthScheme.Logout(token)
		return false, nil
	}
	bsNodeContainer, err = LoadNodeContainer("", BsDefaultName)
	if err != nil {
		return true, err
	}
	tsuruEndpoint, _ := config.GetString("host")
	if !strings.HasPrefix(tsuruEndpoint, "http://") && !strings.HasPrefix(tsuruEndpoint, "https://") {
		tsuruEndpoint = "http://" + tsuruEndpoint
	}
	tsuruEndpoint = strings.TrimRight(tsuruEndpoint, "/") + "/"
	socket, _ := config.GetString("docker:bs:socket")
	image, _ := config.GetString("docker:bs:image")
	if image == "" {
		image = bsDefaultImageName
	}
	bsNodeContainer.Name = BsDefaultName
	bsNodeContainer.Config.Env = append(bsNodeContainer.Config.Env, []string{
		"TSURU_ENDPOINT=" + tsuruEndpoint,
		"HOST_PROC=" + bsHostProc,
		"SYSLOG_LISTEN_ADDRESS=" + fmt.Sprintf("udp://0.0.0.0:%d", container.BsSysLogPort()),
	}...)
	bsNodeContainer.Config.Image = image
	bsNodeContainer.HostConfig.RestartPolicy = docker.AlwaysRestart()
	bsNodeContainer.HostConfig.Privileged = true
	bsNodeContainer.HostConfig.NetworkMode = "host"
	bsNodeContainer.HostConfig.Binds = []string{fmt.Sprintf("/proc:%s:ro", bsHostProc)}
	if socket != "" {
		bsNodeContainer.Config.Env = append(bsNodeContainer.Config.Env, "DOCKER_ENDPOINT=unix:///var/run/docker.sock")
		bsNodeContainer.HostConfig.Binds = append(bsNodeContainer.HostConfig.Binds, fmt.Sprintf("%s:/var/run/docker.sock:rw", socket))
	}
	return true, conf.Save("", bsNodeContainer)
}
