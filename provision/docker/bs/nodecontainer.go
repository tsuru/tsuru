// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision/docker/fix"
	"github.com/tsuru/tsuru/scopedconfig"
)

const (
	nodeContainerCollection = "nodeContainer"
)

type NodeContainerConfig struct {
	Name        string
	PinnedImage string
	Config      docker.Config
	HostConfig  docker.HostConfig
}

func configFor(name string) *scopedconfig.ScopedConfig {
	conf := scopedconfig.FindScopedConfigFor(nodeContainerCollection, name)
	conf.Jsonfy = true
	conf.ShallowMerge = true
	return conf
}

func AddNewContainer(pool string, c *NodeContainerConfig) error {
	if c.Name == "" {
		return errors.New("container config name cannot be empty")
	}
	c.PinnedImage = ""
	conf := configFor(c.Name)
	return conf.Save(pool, c)
}

func EnsureContainersStarted(p DockerProvisioner, w io.Writer) error {
	if w == nil {
		w = ioutil.Discard
	}
	confs, err := scopedconfig.FindAllScopedConfig(nodeContainerCollection)
	if err != nil {
		return err
	}
	cluster := p.Cluster()
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		return err
	}
	errChan := make(chan error, len(nodes))
	wg := sync.WaitGroup{}
	log.Debugf("[node containers] recreating %d containers", len(nodes))
	for i := range nodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			node := &nodes[i]
			pool := node.Metadata["pool"]
			for _, c := range confs {
				c.Jsonfy = true
				c.ShallowMerge = true
				var containerConfig NodeContainerConfig
				confErr := c.Load(pool, &containerConfig)
				if confErr != nil {
					errChan <- confErr
					continue
				}
				log.Debugf("[node containers] recreating container %q in %s [%s]", c.GetName(), node.Address, pool)
				fmt.Fprintf(w, "relaunching node container %q in the node %s [%s]\n", c.GetName(), node.Address, pool)
				confErr = containerConfig.Create(node.Address, pool, p, true)
				if confErr != nil {
					msg := fmt.Sprintf("[node containers] failed to create container in %s [%s]: %s", node.Address, pool, confErr)
					log.Error(msg)
					errChan <- errors.New(msg)
				}
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

func (c *NodeContainerConfig) image() string {
	if c.PinnedImage != "" {
		return c.PinnedImage
	}
	return c.Config.Image
}

func (c *NodeContainerConfig) pullImage(client *docker.Client, poolName string, p DockerProvisioner) error {
	image := c.image()
	var buf bytes.Buffer
	var err error
	pullOpts := docker.PullImageOptions{Repository: image, OutputStream: &buf}
	registryAuth := p.RegistryAuthConfig()
	maxTries := 3
	for ; maxTries > 0; maxTries-- {
		err = client.PullImage(pullOpts, registryAuth)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	if shouldPinImage(image) {
		digest, _ := fix.GetImageDigest(buf.String())
		if digest != "" {
			image = fmt.Sprintf("%s@%s", image, digest)
		}
	}
	c.PinnedImage = image
	conf := configFor(c.Name)
	return conf.SetField(poolName, "pinnedimage", image)
}

func (c *NodeContainerConfig) Create(dockerEndpoint, poolName string, p DockerProvisioner, relaunch bool) error {
	client, err := dockerClient(dockerEndpoint)
	if err != nil {
		return err
	}
	err = c.pullImage(client, poolName, p)
	if err != nil {
		return err
	}
	c.HostConfig.RestartPolicy = docker.AlwaysRestart()
	c.Config.Image = c.image()
	opts := docker.CreateContainerOptions{
		Name:       c.Name,
		HostConfig: &c.HostConfig,
		Config:     &c.Config,
	}
	_, err = client.CreateContainer(opts)
	if relaunch && err == docker.ErrContainerAlreadyExists {
		err = client.RemoveContainer(docker.RemoveContainerOptions{ID: opts.Name, Force: true})
		if err != nil {
			return err
		}
		_, err = client.CreateContainer(opts)
	}
	if err != nil && err != docker.ErrContainerAlreadyExists {
		return err
	}
	err = client.StartContainer(c.Name, &c.HostConfig)
	if _, ok := err.(*docker.ContainerAlreadyRunning); !ok {
		return err
	}
	return nil
}

func shouldPinImage(image string) bool {
	parts := strings.SplitN(image, "/", 3)
	lastPart := parts[len(parts)-1]
	return len(strings.SplitN(lastPart, ":", 2)) < 2
}
