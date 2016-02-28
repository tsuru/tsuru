// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/fix"
)

type DockerProvisioner interface {
	Cluster() *cluster.Cluster
	RegistryAuthConfig() docker.AuthConfiguration
}

const (
	bsUniqueID         = "bs"
	bsDefaultImageName = "tsuru/bs:v1"
)

func EnvListForEndpoint(dockerEndpoint, poolName string) ([]string, error) {
	bsConf, err := provision.FindScopedConfig(bsUniqueID)
	if err != nil {
		return nil, err
	}
	tsuruEndpoint, _ := config.GetString("host")
	if !strings.HasPrefix(tsuruEndpoint, "http://") && !strings.HasPrefix(tsuruEndpoint, "https://") {
		tsuruEndpoint = "http://" + tsuruEndpoint
	}
	tsuruEndpoint = strings.TrimRight(tsuruEndpoint, "/") + "/"
	endpoint := dockerEndpoint
	socket, _ := config.GetString("docker:bs:socket")
	if socket != "" {
		endpoint = "unix:///var/run/docker.sock"
	}
	token, err := getToken(bsConf)
	if err != nil {
		return nil, err
	}
	baseEnvMap := map[string]string{
		"DOCKER_ENDPOINT":       endpoint,
		"TSURU_ENDPOINT":        tsuruEndpoint,
		"TSURU_TOKEN":           token,
		"SYSLOG_LISTEN_ADDRESS": fmt.Sprintf("udp://0.0.0.0:%d", container.BsSysLogPort()),
	}
	var envList []string
	for envName, envValue := range bsConf.PoolEntries(poolName) {
		if _, isBase := baseEnvMap[envName]; isBase {
			continue
		}
		envList = append(envList, fmt.Sprintf("%s=%s", envName, envValue.Value))
	}
	for name, value := range baseEnvMap {
		envList = append(envList, fmt.Sprintf("%s=%s", name, value))
	}
	return envList, nil
}

func getToken(bsConf *provision.ScopedConfig) (string, error) {
	token := bsConf.GetExtraString("token")
	if token != "" {
		return token, nil
	}
	tokenData, err := app.AuthScheme.AppLogin(app.InternalAppName)
	if err != nil {
		return "", err
	}
	token = tokenData.GetValue()
	isSet, err := bsConf.SetExtraAtomic("token", token)
	if isSet {
		return token, nil
	}
	app.AuthScheme.Logout(token)
	if err != nil {
		return "", err
	}
	token = bsConf.GetExtraString("token")
	if token == "" {
		return "", fmt.Errorf("invalid empty bs api token")
	}
	return token, nil
}

func SaveImage(digest string) error {
	bsConf, err := provision.FindScopedConfig(bsUniqueID)
	if err != nil {
		return err
	}
	return bsConf.SetExtra("image", digest)
}

func LoadConfig(pools []string) (*provision.ScopedConfig, error) {
	bsConf, err := provision.FindScopedConfig(bsUniqueID)
	if err != nil {
		return nil, err
	}
	bsConf.FilterPools(pools)
	return bsConf, nil
}

func dockerClient(endpoint string) (*docker.Client, error) {
	client, err := docker.NewClient(endpoint)
	if err != nil {
		return nil, err
	}
	client.HTTPClient = net.Dial5Full300ClientNoKeepAlive
	client.Dialer = net.Dial5Dialer
	return client, nil
}

func getImage(bsConf *provision.ScopedConfig) string {
	image := bsConf.GetExtraString("image")
	if image != "" {
		return image
	}
	image, _ = config.GetString("docker:bs:image")
	if image == "" {
		image = bsDefaultImageName
	}
	return image
}

func createContainer(dockerEndpoint, poolName string, p DockerProvisioner, relaunch bool) error {
	client, err := dockerClient(dockerEndpoint)
	if err != nil {
		return err
	}
	bsConf, err := provision.FindScopedConfig(bsUniqueID)
	if err != nil {
		return err
	}
	bsImage := getImage(bsConf)
	err = pullBsImage(bsImage, dockerEndpoint, p)
	if err != nil {
		return err
	}
	hostConfig := docker.HostConfig{
		RestartPolicy: docker.AlwaysRestart(),
		Privileged:    true,
		NetworkMode:   "host",
	}
	socket, _ := config.GetString("docker:bs:socket")
	if socket != "" {
		hostConfig.Binds = []string{fmt.Sprintf("%s:/var/run/docker.sock:rw", socket)}
	}
	env, err := EnvListForEndpoint(dockerEndpoint, poolName)
	if err != nil {
		return err
	}
	opts := docker.CreateContainerOptions{
		Name:       "big-sibling",
		HostConfig: &hostConfig,
		Config: &docker.Config{
			Image: bsImage,
			Env:   env,
		},
	}
	container, err := client.CreateContainer(opts)
	if relaunch && err == docker.ErrContainerAlreadyExists {
		err = client.RemoveContainer(docker.RemoveContainerOptions{ID: opts.Name, Force: true})
		if err != nil {
			return err
		}
		container, err = client.CreateContainer(opts)
	}
	if err != nil && err != docker.ErrContainerAlreadyExists {
		return err
	}
	if container == nil {
		container, err = client.InspectContainer("big-sibling")
		if err != nil {
			return err
		}
	}
	err = client.StartContainer(container.ID, &hostConfig)
	if _, ok := err.(*docker.ContainerAlreadyRunning); !ok {
		return err
	}
	return nil
}

func pullWithRetry(maxTries int, image, dockerEndpoint string, p DockerProvisioner) (string, error) {
	client, err := dockerClient(dockerEndpoint)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	pullOpts := docker.PullImageOptions{Repository: image, OutputStream: &buf}
	registryAuth := p.RegistryAuthConfig()
	for ; maxTries > 0; maxTries-- {
		err = client.PullImage(pullOpts, registryAuth)
		if err == nil {
			return buf.String(), nil
		}
	}
	return "", err
}

func pullBsImage(image, dockerEndpoint string, p DockerProvisioner) error {
	output, err := pullWithRetry(3, image, dockerEndpoint, p)
	if err != nil {
		return err
	}
	if shouldPinBsImage(image) {
		digest, _ := fix.GetImageDigest(output)
		if digest != "" {
			image = fmt.Sprintf("%s@%s", image, digest)
		}
	}
	return SaveImage(image)
}

func shouldPinBsImage(image string) bool {
	parts := strings.SplitN(image, "/", 3)
	lastPart := parts[len(parts)-1]
	return len(strings.SplitN(lastPart, ":", 2)) < 2
}

// RecreateContainers relaunch all bs containers in the cluster for the given
// DockerProvisioner, logging progress to the given writer.
//
// It assumes that the given writer is thread safe.
func RecreateContainers(p DockerProvisioner, w io.Writer) error {
	cluster := p.Cluster()
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		return err
	}
	errChan := make(chan error, len(nodes))
	wg := sync.WaitGroup{}
	log.Debugf("[bs containers] recreating %d containers", len(nodes))
	for i := range nodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			node := &nodes[i]
			pool := node.Metadata["pool"]
			log.Debugf("[bs containers] recreating container in %s [%s]", node.Address, pool)
			fmt.Fprintf(w, "relaunching bs container in the node %s [%s]\n", node.Address, pool)
			createErr := createContainer(node.Address, pool, p, true)
			if createErr != nil {
				msg := fmt.Sprintf("[bs containers] failed to create container in %s [%s]: %s", node.Address, pool, createErr)
				log.Error(msg)
				errChan <- errors.New(msg)
			}
		}(i)
	}
	wg.Wait()
	close(errChan)
	var allErrors []string
	for err = range errChan {
		allErrors = append(allErrors, err.Error())
	}
	if len(allErrors) == 0 {
		return nil
	}
	return fmt.Errorf("multiple errors: %s", strings.Join(allErrors, ", "))
}

type ClusterHook struct {
	Provisioner DockerProvisioner
}

func (h *ClusterHook) BeforeCreateContainer(node cluster.Node) error {
	err := createContainer(node.Address, node.Metadata["pool"], h.Provisioner, false)
	if err != nil {
		return err
	}
	return nil
}
