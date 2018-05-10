// Copyright 2017 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/log"
)

type Container struct {
	Id   string `bson:"_id"`
	Host string
}

// CreateContainer creates a container in the specified node. If no node is
// specified, it will create the container in a node selected by the scheduler.
//
// It returns the container, or an error, in case of failures.
func (c *Cluster) CreateContainer(opts docker.CreateContainerOptions, inactivityTimeout time.Duration, nodes ...string) (string, *docker.Container, error) {
	return c.CreateContainerSchedulerOpts(opts, nil, inactivityTimeout, nodes...)
}

// Similar to CreateContainer but allows arbritary options to be passed to
// the scheduler.
func (c *Cluster) CreateContainerSchedulerOpts(opts docker.CreateContainerOptions, schedulerOpts SchedulerOptions, inactivityTimeout time.Duration, nodes ...string) (string, *docker.Container, error) {
	return c.CreateContainerPullOptsSchedulerOpts(opts, docker.PullImageOptions{
		Repository:        opts.Config.Image,
		InactivityTimeout: inactivityTimeout,
		Context:           opts.Context,
	}, docker.AuthConfiguration{}, schedulerOpts, nodes...)
}

// Similar to CreateContainer but allows arbritary options to be passed to
// the scheduler and to the pull image call.
func (c *Cluster) CreateContainerPullOptsSchedulerOpts(opts docker.CreateContainerOptions, pullOpts docker.PullImageOptions, pullAuth docker.AuthConfiguration, schedulerOpts SchedulerOptions, nodes ...string) (string, *docker.Container, error) {
	var (
		addr      string
		container *docker.Container
		err       error
	)
	useScheduler := len(nodes) == 0
	maxTries := 5
	for ; maxTries > 0; maxTries-- {
		if opts.Context != nil {
			select {
			case <-opts.Context.Done():
				return "", nil, opts.Context.Err()
			default:
			}
		}
		if useScheduler {
			node, scheduleErr := c.scheduler.Schedule(c, &opts, schedulerOpts)
			if scheduleErr != nil {
				if err != nil {
					scheduleErr = fmt.Errorf("Error in scheduler after previous errors (%s) trying to create container: %s", err.Error(), scheduleErr.Error())
				}
				return addr, nil, scheduleErr
			}
			addr = node.Address
		} else {
			addr = nodes[rand.Intn(len(nodes))]
		}
		if addr == "" {
			return addr, nil, errors.New("CreateContainer needs a non empty node addr")
		}
		err = c.runHookForAddr(HookEventBeforeContainerCreate, addr)
		if err != nil {
			log.Errorf("Error in before create container hook in node %q: %s. Trying again in another node...", addr, err)
		}
		if err == nil {
			container, err = c.createContainerInNode(opts, pullOpts, pullAuth, addr)
			if err == nil {
				c.handleNodeSuccess(addr)
				break
			}
			log.Errorf("Error trying to create container in node %q: %s. Trying again in another node...", addr, err.Error())
		}
		shouldIncrementFailures := false
		isCreateContainerErr := false
		baseErr := err
		if nodeErr, ok := baseErr.(DockerNodeError); ok {
			isCreateContainerErr = nodeErr.cmd == "createContainer"
			baseErr = nodeErr.BaseError()
		}
		if urlErr, ok := baseErr.(*url.Error); ok {
			baseErr = urlErr.Err
		}
		_, isNetErr := baseErr.(net.Error)
		if isNetErr || isCreateContainerErr || baseErr == docker.ErrConnectionRefused {
			shouldIncrementFailures = true
		}
		c.handleNodeError(addr, err, shouldIncrementFailures)
		if !useScheduler {
			return addr, nil, err
		}
	}
	if err != nil {
		return addr, nil, fmt.Errorf("CreateContainer: maximum number of tries exceeded, last error: %s", err.Error())
	}
	err = c.storage().StoreContainer(container.ID, addr)
	return addr, container, err
}

func (c *Cluster) createContainerInNode(opts docker.CreateContainerOptions, pullOpts docker.PullImageOptions, pullAuth docker.AuthConfiguration, nodeAddress string) (*docker.Container, error) {
	registryServer, _ := parseImageRegistry(opts.Config.Image)
	err := c.PullImage(pullOpts, pullAuth, nodeAddress)
	if err != nil {
		if registryServer != "" {
			return nil, err
		}
	}
	node, err := c.getNodeByAddr(nodeAddress)
	if err != nil {
		return nil, err
	}
	cont, err := node.CreateContainer(opts)
	return cont, wrapErrorWithCmd(node, err, "createContainer")
}

// InspectContainer returns information about a container by its ID, getting
// the information from the right node.
func (c *Cluster) InspectContainer(id string) (*docker.Container, error) {
	node, err := c.getNodeForContainer(id)
	if err != nil {
		return nil, err
	}
	cont, err := node.InspectContainer(id)
	return cont, wrapError(node, err)
}

// KillContainer kills a container, returning an error in case of failure.
func (c *Cluster) KillContainer(opts docker.KillContainerOptions) error {
	node, err := c.getNodeForContainer(opts.ID)
	if err != nil {
		return err
	}
	return wrapError(node, node.KillContainer(opts))
}

// ListContainers returns a slice of all containers in the cluster matching the
// given criteria.
func (c *Cluster) ListContainers(opts docker.ListContainersOptions) ([]docker.APIContainers, error) {
	nodes, err := c.Nodes()
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	result := make(chan []docker.APIContainers, len(nodes))
	errs := make(chan error, len(nodes))
	for _, n := range nodes {
		wg.Add(1)
		client, _ := c.getNodeByAddr(n.Address)
		go func(n node) {
			defer wg.Done()
			if containers, err := n.ListContainers(opts); err != nil {
				errs <- wrapError(n, err)
			} else {
				result <- containers
			}
		}(client)
	}
	wg.Wait()
	var group []docker.APIContainers
	for {
		select {
		case containers := <-result:
			group = append(group, containers...)
		case err = <-errs:
		default:
			return group, err
		}
	}
}

// RemoveContainer removes a container from the cluster.
func (c *Cluster) RemoveContainer(opts docker.RemoveContainerOptions) error {
	return c.removeFromStorage(opts)
}

func (c *Cluster) removeFromStorage(opts docker.RemoveContainerOptions) error {
	node, err := c.getNodeForContainer(opts.ID)
	if err != nil {
		return err
	}
	err = node.RemoveContainer(opts)
	if err != nil {
		_, isNoSuchContainer := err.(*docker.NoSuchContainer)
		if !isNoSuchContainer {
			return wrapError(node, err)
		}
	}
	return c.storage().RemoveContainer(opts.ID)
}

func (c *Cluster) StartContainer(id string, hostConfig *docker.HostConfig) error {
	node, err := c.getNodeForContainer(id)
	if err != nil {
		return err
	}
	err = node.StartContainer(id, hostConfig)
	if err != nil {
		switch err.(type) {
		case *docker.NoSuchContainer:
		case *docker.ContainerAlreadyRunning:
		default:
			c.handleNodeError(node.addr, err, false)
		}
	}
	return wrapError(node, err)
}

// StopContainer stops a container, killing it after the given timeout, if it
// fails to stop nicely.
func (c *Cluster) StopContainer(id string, timeout uint) error {
	node, err := c.getNodeForContainer(id)
	if err != nil {
		return err
	}
	return wrapError(node, node.StopContainer(id, timeout))
}

// RestartContainer restarts a container, killing it after the given timeout,
// if it fails to stop nicely.
func (c *Cluster) RestartContainer(id string, timeout uint) error {
	node, err := c.getNodeForContainer(id)
	if err != nil {
		return err
	}
	return wrapError(node, node.RestartContainer(id, timeout))
}

// PauseContainer changes the container to the paused state.
func (c *Cluster) PauseContainer(id string) error {
	node, err := c.getNodeForContainer(id)
	if err != nil {
		return err
	}
	return wrapError(node, node.PauseContainer(id))
}

// UnpauseContainer removes the container from the paused state.
func (c *Cluster) UnpauseContainer(id string) error {
	node, err := c.getNodeForContainer(id)
	if err != nil {
		return err
	}
	return wrapError(node, node.UnpauseContainer(id))
}

// WaitContainer blocks until the given container stops, returning the exit
// code of the container command.
func (c *Cluster) WaitContainer(id string) (int, error) {
	node, err := c.getNodeForContainer(id)
	if err != nil {
		return -1, err
	}
	node.setPersistentClient()
	code, err := node.WaitContainer(id)
	return code, wrapError(node, err)
}

// AttachToContainer attaches to a container, using the given options.
func (c *Cluster) AttachToContainer(opts docker.AttachToContainerOptions) error {
	node, err := c.getNodeForContainer(opts.Container)
	if err != nil {
		return err
	}
	node.setPersistentClient()
	return wrapError(node, node.AttachToContainer(opts))
}

// AttachToContainerNonBlocking attaches to a container and returns a docker.CloseWaiter, using given options.
func (c *Cluster) AttachToContainerNonBlocking(opts docker.AttachToContainerOptions) (docker.CloseWaiter, error) {
	node, err := c.getNodeForContainer(opts.Container)
	if err != nil {
		return nil, err
	}
	node.setPersistentClient()
	return node.AttachToContainerNonBlocking(opts)
}

// Logs retrieves the logs of the specified container.
func (c *Cluster) Logs(opts docker.LogsOptions) error {
	node, err := c.getNodeForContainer(opts.Container)
	if err != nil {
		return err
	}
	return wrapError(node, node.Logs(opts))
}

// CommitContainer commits a container and returns the image id.
func (c *Cluster) CommitContainer(opts docker.CommitContainerOptions) (*docker.Image, error) {
	node, err := c.getNodeForContainer(opts.Container)
	if err != nil {
		return nil, err
	}
	node.setPersistentClient()
	image, err := node.CommitContainer(opts)
	if err != nil {
		return nil, wrapError(node, err)
	}
	key := imageKey(opts.Repository, opts.Tag)
	if key != "" {
		err = c.storage().StoreImage(key, image.ID, node.addr)
		if err != nil {
			return nil, err
		}
	}
	return image, nil
}

// ExportContainer exports a container as a tar and writes
// the result in out.
func (c *Cluster) ExportContainer(opts docker.ExportContainerOptions) error {
	node, err := c.getNodeForContainer(opts.ID)
	if err != nil {
		return err
	}
	return wrapError(node, node.ExportContainer(opts))
}

// TopContainer returns information about running processes inside a container
// by its ID, getting the information from the right node.
func (c *Cluster) TopContainer(id string, psArgs string) (docker.TopResult, error) {
	node, err := c.getNodeForContainer(id)
	if err != nil {
		return docker.TopResult{}, err
	}
	result, err := node.TopContainer(id, psArgs)
	return result, wrapError(node, err)
}

func (c *Cluster) getNodeForContainer(container string) (node, error) {
	addr, err := c.storage().RetrieveContainer(container)
	if err != nil {
		return node{}, err
	}
	return c.getNodeByAddr(addr)
}

func (c *Cluster) getNodeForExec(execID string) (node, error) {
	containerID, err := c.storage().RetrieveExec(execID)
	if err != nil {
		return node{}, err
	}
	return c.getNodeForContainer(containerID)
}

func (c *Cluster) CreateExec(opts docker.CreateExecOptions) (*docker.Exec, error) {
	node, err := c.getNodeForContainer(opts.Container)
	if err != nil {
		return nil, err
	}
	exec, err := node.CreateExec(opts)
	if err != nil {
		return nil, wrapError(node, err)
	}
	err = c.storage().StoreExec(exec.ID, opts.Container)
	return exec, err
}

func (c *Cluster) StartExec(execId string, opts docker.StartExecOptions) error {
	node, err := c.getNodeForExec(execId)
	if err != nil {
		return err
	}
	node.setPersistentClient()
	return wrapError(node, node.StartExec(execId, opts))
}

func (c *Cluster) ResizeExecTTY(execId string, height, width int) error {
	node, err := c.getNodeForExec(execId)
	if err != nil {
		return err
	}
	return wrapError(node, node.ResizeExecTTY(execId, height, width))
}

func (c *Cluster) InspectExec(execId string) (*docker.ExecInspect, error) {
	node, err := c.getNodeForExec(execId)
	if err != nil {
		return nil, err
	}
	execInspect, err := node.InspectExec(execId)
	if err != nil {
		return nil, wrapError(node, err)
	}
	return execInspect, nil
}

func (c *Cluster) UploadToContainer(containerId string, opts docker.UploadToContainerOptions) error {
	node, err := c.getNodeForContainer(containerId)
	if err != nil {
		return err
	}
	return node.UploadToContainer(containerId, opts)
}

func (c *Cluster) DownloadFromContainer(containerId string, opts docker.DownloadFromContainerOptions) error {
	node, err := c.getNodeForContainer(containerId)
	if err != nil {
		return err
	}
	return node.DownloadFromContainer(containerId, opts)
}

func (c *Cluster) ResizeContainerTTY(containerId string, height, width int) error {
	node, err := c.getNodeForContainer(containerId)
	if err != nil {
		return err
	}
	return wrapError(node, node.ResizeContainerTTY(containerId, height, width))
}
