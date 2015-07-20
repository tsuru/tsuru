// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2"
)

const runBsTaskName = "run-bs"

var digestRegexp = regexp.MustCompile(`(?m)^Digest: (.*)$`)

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
	rawMetadata := params["metadata"].(monsterqueue.JobParams)
	metadata := make(map[string]string, len(rawMetadata))
	for key, value := range rawMetadata {
		metadata[key] = value.(string)
	}
	err = createBsContainer(dockerEndpoint, metadata["pool"])
	if err != nil {
		job.Error(err)
		t.destroyMachine(machineID)
		return
	}
	err = mainDockerProvisioner.getCluster().Register(cluster.Node{Address: dockerEndpoint, Metadata: metadata})
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

func createBsContainer(dockerEndpoint, poolName string) error {
	client, err := docker.NewClient(dockerEndpoint)
	if err != nil {
		return err
	}
	bsConf, err := loadBsConfig()
	if err != nil {
		if err != mgo.ErrNotFound {
			return err
		}
		bsConf = &bsConfig{}
	}
	bsImage := bsConf.getImage()
	err = pullBsImage(bsImage, dockerEndpoint)
	if err != nil {
		return err
	}
	hostConfig := docker.HostConfig{
		RestartPolicy: docker.AlwaysRestart(),
		Privileged:    true,
		NetworkMode:   "host",
	}
	sysLogExternalPort := getBsSysLogPort()
	socket, _ := config.GetString("docker:bs:socket")
	if socket != "" {
		hostConfig.Binds = []string{fmt.Sprintf("%s:/var/run/docker.sock:rw", socket)}
	}
	hostConfig.PortBindings = map[docker.Port][]docker.PortBinding{
		docker.Port("514/udp"): {
			docker.PortBinding{
				HostIP:   "0.0.0.0",
				HostPort: strconv.Itoa(sysLogExternalPort),
			},
		},
	}
	env, err := bsConf.envListForEndpoint(dockerEndpoint, poolName)
	if err != nil {
		return err
	}
	opts := docker.CreateContainerOptions{
		Name:       "big-sibling",
		HostConfig: &hostConfig,
		Config: &docker.Config{
			Image: bsImage,
			Env:   env,
			ExposedPorts: map[docker.Port]struct{}{
				docker.Port("514/udp"): {},
			},
		},
	}
	container, err := client.CreateContainer(opts)
	if err == docker.ErrContainerAlreadyExists {
		err = client.RemoveContainer(docker.RemoveContainerOptions{ID: opts.Name, Force: true})
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

func pullBsImage(image, dockerEndpoint string) error {
	client, err := docker.NewClient(dockerEndpoint)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	pullOpts := docker.PullImageOptions{Repository: image, OutputStream: &buf}
	err = client.PullImage(pullOpts, getRegistryAuthConfig())
	if err != nil {
		return err
	}
	if shouldPinBsImage(image) {
		match := digestRegexp.FindAllStringSubmatch(buf.String(), 1)
		if len(match) > 0 {
			image += "@" + match[0][1]
		}
	}
	return saveBsImage(image)
}

func shouldPinBsImage(image string) bool {
	parts := strings.SplitN(image, "/", 3)
	lastPart := parts[len(parts)-1]
	return len(strings.SplitN(lastPart, ":", 2)) < 2
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
