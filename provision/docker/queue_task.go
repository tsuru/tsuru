// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
)

const runBsTaskName = "run-bs"

type runBs struct{}

func (runBs) Name() string {
	return runBsTaskName
}

func (t runBs) Run(job monsterqueue.Job) {
	params := job.Parameters()
	dockerEndpoint := params["endpoint"].(string)
	machineID := params["machine"].(string)
	err := t.waitDocker(dockerEndpoint)
	if err != nil {
		job.Error(err)
		return
	}
	err = t.createBsContainer(dockerEndpoint)
	if err != nil {
		job.Error(err)
		t.destroyMachine(machineID)
		return
	}
	rawMetadata := params["metadata"].(monsterqueue.JobParams)
	metadata := make(map[string]string, len(rawMetadata))
	for key, value := range rawMetadata {
		metadata[key] = value.(string)
	}
	_, err = mainDockerProvisioner.getCluster().Register(dockerEndpoint, metadata)
	if err != nil {
		job.Error(err)
		t.destroyMachine(machineID)
		return
	}
	job.Success(nil)
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
	bsImage, _ := config.GetString("docker:bs:image")
	if bsImage == "" {
		bsImage = "tsuru/bs"
	}
	tsuruEndpoint, _ := config.GetString("host")
	if !strings.HasPrefix(tsuruEndpoint, "http://") && !strings.HasPrefix(tsuruEndpoint, "https://") {
		tsuruEndpoint = "http://" + tsuruEndpoint
	}
	interval, _ := config.GetInt("docker:bs:reporter-interval")
	tsuruEndpoint = strings.TrimRight(tsuruEndpoint, "/") + "/"
	token, err := app.AuthScheme.AppLogin(app.InternalAppName)
	if err != nil {
		return err
	}
	sentinelEnvVar := "TSURU_APPNAME"
	hostConfig := docker.HostConfig{RestartPolicy: docker.AlwaysRestart()}
	endpoint := dockerEndpoint
	socket, _ := config.GetString("docker:bs:socket")
	if socket != "" {
		hostConfig.Binds = []string{fmt.Sprintf("%s:/var/run/docker.sock:rw", socket)}
		endpoint = "unix:///var/run/docker.sock"
	}
	sysLogInternalPort, _ := config.GetInt("docker:bs:syslog-internal-port")
	if sysLogInternalPort == 0 {
		sysLogInternalPort = 514
	}
	sysLogExternalPort := getBsSysLogPort()
	hostConfig.PortBindings = map[docker.Port][]docker.PortBinding{
		docker.Port(strconv.Itoa(sysLogInternalPort) + "/udp"): {
			docker.PortBinding{
				HostIP:   "0.0.0.0",
				HostPort: strconv.Itoa(sysLogExternalPort),
			},
		},
	}
	env := []string{
		"DOCKER_ENDPOINT=" + endpoint,
		"TSURU_ENDPOINT=" + tsuruEndpoint,
		"TSURU_TOKEN=" + token.GetValue(),
		"TSURU_SENTINEL_ENV_VAR=" + sentinelEnvVar,
		"STATUS_INTERVAL=" + strconv.Itoa(interval),
		"SYSLOG_LISTEN_ADDRESS=" + "udp://0.0.0.0:" + strconv.Itoa(sysLogInternalPort),
	}
	opts := docker.CreateContainerOptions{
		Name:       "big-sibling",
		HostConfig: &hostConfig,
		Config: &docker.Config{
			Image: bsImage,
			Env:   env,
			ExposedPorts: map[docker.Port]struct{}{
				docker.Port(strconv.Itoa(sysLogExternalPort) + "/udp"): {},
			},
		},
	}
	container, err := client.CreateContainer(opts)
	if err == docker.ErrNoSuchImage {
		pullOpts := docker.PullImageOptions{Repository: bsImage}
		err = client.PullImage(pullOpts, getRegistryAuthConfig())
		if err != nil {
			return err
		}
		container, err = client.CreateContainer(opts)
	}
	if err != nil {
		return err
	}
	return client.StartContainer(container.ID, &hostConfig)
}

func getBsSysLogPort() int {
	bsPort, _ := config.GetInt("docker:bs:syslog-external-port")
	if bsPort == 0 {
		bsPort = 1514
	}
	return bsPort
}

func (runBs) destroyMachine(id string) {
	if id != "" {
		machine, err := iaas.FindMachineById(id)
		if err != nil {
			log.Errorf("failed to remove machine %q: %s", id, err)
			return
		}
		err = machine.Destroy()
		if err != nil {
			log.Errorf("failed to remove machine %q: %s", id, err)
			return
		}
	}
}
