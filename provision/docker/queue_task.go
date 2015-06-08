// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/app"
)

const runBsTaskName = "run-bs"

type runBs struct{}

func (runBs) Name() string {
	return runBsTaskName
}

func (t runBs) Run(job monsterqueue.Job) {
	params := job.Parameters()
	dockerEndpoint := params["endpoint"].(string)
	err := t.waitDocker(dockerEndpoint)
	if err != nil {
		job.Error(err)
		return
	}
	err = t.createBsContainer(dockerEndpoint)
	if err != nil {
		job.Error(err)
	}
}

func (runBs) waitDocker(endpoint string) error {
	client, err := docker.NewClient(endpoint)
	if err != nil {
		return err
	}
	timeout, _ := config.GetInt("docker:api-timeout")
	if timeout == 0 {
		timeout = 600
	}
	timeoutChan := time.After(time.Duration(timeout) * time.Second)
	pong := make(chan error, 1)
	go func() {
		for {
			err := client.Ping()
			if err == nil {
				pong <- nil
				return
			}
			if e, ok := err.(*docker.Error); ok && e.Status > 499 {
				pong <- err
				return
			}
		}
	}()
	select {
	case err := <-pong:
		return err
	case <-timeoutChan:
		return fmt.Errorf("Docker API at %q didn't respond after %d seconds", endpoint, timeout)
	}
}

func (runBs) createBsContainer(dockerEndpoint string) error {
	client, err := docker.NewClient(dockerEndpoint)
	if err != nil {
		return err
	}
	bsImage, _ := config.GetString("docker:bs-image")
	if bsImage == "" {
		bsImage = "tsuru/bs"
	}
	tsuruEndpoint, _ := config.GetString("host")
	if !strings.HasPrefix(tsuruEndpoint, "http://") && !strings.HasPrefix(tsuruEndpoint, "https://") {
		tsuruEndpoint = "http://" + tsuruEndpoint
	}
	tsuruEndpoint = strings.TrimRight(tsuruEndpoint, "/") + "/"
	token, err := app.AuthScheme.AppLogin(app.InternalAppName)
	if err != nil {
		return err
	}
	sentinelEnvVar := "TSURU_APP_NAME"
	env := []string{
		"DOCKER_ENDPOINT=" + dockerEndpoint,
		"TSURU_ENDPOINT=" + tsuruEndpoint,
		"TSURU_TOKEN=" + token.GetValue(),
		"TSURU_SENTINEL_ENV_VAR=" + sentinelEnvVar,
	}
	opts := docker.CreateContainerOptions{
		Name: "big-sibling",
		Config: &docker.Config{
			Image: bsImage,
			Env:   env,
		},
	}
	_, err = client.CreateContainer(opts)
	if err == docker.ErrNoSuchImage {
		pullOpts := docker.PullImageOptions{Repository: bsImage}
		err = client.PullImage(pullOpts, getRegistryAuthConfig())
		if err != nil {
			return err
		}
		_, err = client.CreateContainer(opts)
	}
	return err
}
