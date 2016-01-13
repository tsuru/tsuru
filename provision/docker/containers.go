// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
)

type appLocker struct {
	mut      sync.Mutex
	refCount map[string]int
}

func (l *appLocker) Lock(appName string) bool {
	l.mut.Lock()
	defer l.mut.Unlock()
	if l.refCount == nil {
		l.refCount = make(map[string]int)
	}
	if l.refCount[appName] > 0 {
		l.refCount[appName]++
		return true
	}
	ok, err := app.AcquireApplicationLock(appName, app.InternalAppName, "container-move")
	if err != nil || !ok {
		return false
	}
	l.refCount[appName]++
	return true
}

func (l *appLocker) Unlock(appName string) {
	l.mut.Lock()
	defer l.mut.Unlock()
	if l.refCount == nil {
		return
	}
	l.refCount[appName]--
	if l.refCount[appName] <= 0 {
		l.refCount[appName] = 0
		routesRebuildOrEnqueue(appName)
		app.ReleaseApplicationLock(appName)
	}
}

var containerMovementErr = errors.New("Error moving some containers.")

func (p *dockerProvisioner) HandleMoveErrors(moveErrors chan error, writer io.Writer) error {
	hasError := false
	for err := range moveErrors {
		errMsg := fmt.Sprintf("Error moving container: %s", err.Error())
		log.Error(errMsg)
		fmt.Fprintf(writer, "%s\n", errMsg)
		hasError = true
	}
	if hasError {
		return containerMovementErr
	}
	return nil
}

func (p *dockerProvisioner) runReplaceUnitsPipeline(w io.Writer, a provision.App, toAdd map[string]*containersToAdd, toRemoveContainers []container.Container, imageId string, toHosts ...string) ([]container.Container, error) {
	var toHost string
	if len(toHosts) > 0 {
		toHost = toHosts[0]
	}
	if w == nil {
		w = ioutil.Discard
	}
	args := changeUnitsPipelineArgs{
		app:         a,
		toAdd:       toAdd,
		toRemove:    toRemoveContainers,
		toHost:      toHost,
		writer:      w,
		imageId:     imageId,
		provisioner: p,
	}
	var pipeline *action.Pipeline
	if p.isDryMode {
		pipeline = action.NewPipeline(
			&provisionAddUnitsToHost,
			&provisionRemoveOldUnits,
		)
	} else {
		pipeline = action.NewPipeline(
			&provisionAddUnitsToHost,
			&bindAndHealthcheck,
			&addNewRoutes,
			&removeOldRoutes,
			&updateAppImage,
			&provisionRemoveOldUnits,
			&provisionUnbindOldUnits,
		)
	}
	err := pipeline.Execute(args)
	if err != nil {
		return nil, err
	}
	return pipeline.Result().([]container.Container), nil
}

func (p *dockerProvisioner) runCreateUnitsPipeline(w io.Writer, a provision.App, toAdd map[string]*containersToAdd, imageId string) ([]container.Container, error) {
	if w == nil {
		w = ioutil.Discard
	}
	args := changeUnitsPipelineArgs{
		app:         a,
		toAdd:       toAdd,
		writer:      w,
		imageId:     imageId,
		provisioner: p,
	}
	pipeline := action.NewPipeline(
		&provisionAddUnitsToHost,
		&bindAndHealthcheck,
		&addNewRoutes,
		&updateAppImage,
	)
	err := pipeline.Execute(args)
	if err != nil {
		return nil, err
	}
	return pipeline.Result().([]container.Container), nil
}

func (p *dockerProvisioner) MoveOneContainer(c container.Container, toHost string, errors chan error, wg *sync.WaitGroup, writer io.Writer, locker container.AppLocker) container.Container {
	if wg != nil {
		defer wg.Done()
	}
	locked := locker.Lock(c.AppName)
	if !locked {
		errors <- fmt.Errorf("couldn't move %s, unable to lock %q", c.ID, c.AppName)
		return container.Container{}
	}
	defer locker.Unlock(c.AppName)
	a, err := app.GetByName(c.AppName)
	if err != nil {
		errors <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("error getting app %q for unit %s", c.AppName, c.ID),
		}
		return container.Container{}
	}
	imageId, err := appCurrentImageName(a.GetName())
	if err != nil {
		errors <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("error getting app %q image name for unit %s", c.AppName, c.ID),
		}
		return container.Container{}
	}
	var destHosts []string
	var suffix string
	if toHost != "" {
		destHosts = []string{toHost}
		suffix = " -> " + toHost
	}
	if !p.isDryMode {
		fmt.Fprintf(writer, "Moving unit %s for %q from %s%s...\n", c.ID, c.AppName, c.HostAddr, suffix)
	}
	toAdd := map[string]*containersToAdd{c.ProcessName: {Quantity: 1, Status: provision.Status(c.Status)}}
	addedContainers, err := p.runReplaceUnitsPipeline(nil, a, toAdd, []container.Container{c}, imageId, destHosts...)
	if err != nil {
		errors <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("Error moving unit %s", c.ID),
		}
		return container.Container{}
	}
	prefix := "Moved unit"
	if p.isDryMode {
		prefix = "Would move unit"
	}
	fmt.Fprintf(writer, "%s %s -> %s for %q from %s -> %s\n", prefix, c.ID, addedContainers[0].ID, c.AppName, c.HostAddr, addedContainers[0].HostAddr)
	return addedContainers[0]
}

func (p *dockerProvisioner) moveContainer(contId string, toHost string, writer io.Writer) (container.Container, error) {
	cont, err := p.GetContainer(contId)
	if err != nil {
		return container.Container{}, err
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	moveErrors := make(chan error, 1)
	locker := &appLocker{}
	createdContainer := p.MoveOneContainer(*cont, toHost, moveErrors, &wg, writer, locker)
	close(moveErrors)
	return createdContainer, p.HandleMoveErrors(moveErrors, writer)
}

func (p *dockerProvisioner) moveContainerList(containers []container.Container, toHost string, writer io.Writer) error {
	locker := &appLocker{}
	moveErrors := make(chan error, len(containers))
	wg := sync.WaitGroup{}
	wg.Add(len(containers))
	for _, c := range containers {
		go p.MoveOneContainer(c, toHost, moveErrors, &wg, writer, locker)
	}
	go func() {
		wg.Wait()
		close(moveErrors)
	}()
	return p.HandleMoveErrors(moveErrors, writer)
}

func (p *dockerProvisioner) MoveContainers(fromHost, toHost string, writer io.Writer) error {
	containers, err := p.listContainersByHost(fromHost)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		fmt.Fprintf(writer, "No units to move in %s\n", fromHost)
		return nil
	}
	fmt.Fprintf(writer, "Moving %d units...\n", len(containers))
	return p.moveContainerList(containers, toHost, writer)
}

func (p *dockerProvisioner) moveContainersFromHosts(fromHosts []string, toHost string, writer io.Writer) error {
	var allContainers []container.Container
	for _, host := range fromHosts {
		containers, err := p.listContainersByHost(host)
		if err != nil {
			return err
		}
		allContainers = append(allContainers, containers...)
	}
	if len(allContainers) == 0 {
		fmt.Fprintf(writer, "No units to move in hosts %s\n", strings.Join(fromHosts, ", "))
		return nil
	}
	fmt.Fprintf(writer, "Moving %d units...\n", len(allContainers))
	return p.moveContainerList(allContainers, toHost, writer)
}

type hostWithContainers struct {
	HostAddr   string `bson:"_id"`
	Count      int
	Containers []container.Container
}

func minCountHost(hosts []hostWithContainers) *hostWithContainers {
	var minCountHost *hostWithContainers
	minCount := math.MaxInt32
	for i, dest := range hosts {
		if dest.Count < minCount {
			minCount = dest.Count
			minCountHost = &hosts[i]
		}
	}
	return minCountHost
}

func (p *dockerProvisioner) rebalanceContainersByFilter(writer io.Writer, appFilter []string, metadataFilter map[string]string, dryRun bool) (*dockerProvisioner, error) {
	var hostsFilter []string
	if metadataFilter != nil {
		nodes, err := p.cluster.UnfilteredNodesForMetadata(metadataFilter)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			hostsFilter = append(hostsFilter, net.URLToHost(n.Address))
		}
		if len(hostsFilter) == 0 {
			fmt.Fprintf(writer, "No hosts matching metadata filters\n")
			return nil, nil
		}
	}
	containers, err := p.listContainersByAppAndHost(appFilter, hostsFilter)
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		fmt.Fprintf(writer, "No containers found to rebalance\n")
		return nil, nil
	}
	if dryRun {
		p, err = p.dryMode(containers)
		if err != nil {
			return nil, err
		}
		defer p.stopDryMode()
	} else {
		// Create isolated provisioner, this allow us to use modify the
		// scheduler to exclude old containers.
		p, err = p.cloneProvisioner(containers)
		if err != nil {
			return nil, err
		}
	}
	fmt.Fprintf(writer, "Rebalancing %d units...\n", len(containers))
	return p, p.moveContainerList(containers, "", writer)
}

func (p *dockerProvisioner) rebalanceContainersByHost(address string, w io.Writer) error {
	containers, err := p.listContainersByHost(address)
	if err != nil {
		return err
	}
	return p.moveContainerList(containers, "", w)
}

func (p *dockerProvisioner) rebalanceContainers(writer io.Writer, dryRun bool) error {
	_, err := p.rebalanceContainersByFilter(writer, nil, nil, dryRun)
	return err
}

func (p *dockerProvisioner) runCommandInContainer(image string, command string, app provision.App) (bytes.Buffer, error) {
	var output bytes.Buffer
	user, err := config.GetString("docker:user")
	if err != nil {
		user, _ = config.GetString("docker:ssh:user")
	}
	createOptions := docker.CreateContainerOptions{
		Config: &docker.Config{
			AttachStdout: true,
			AttachStderr: true,
			User:         user,
			Image:        image,
			Entrypoint:   []string{"/bin/bash", "-c"},
			Cmd:          []string{command},
		},
	}
	cluster := p.Cluster()
	_, cont, err := cluster.CreateContainerSchedulerOpts(createOptions, []string{app.GetName(), ""})
	if err != nil {
		return output, err
	}
	attachOptions := docker.AttachToContainerOptions{
		Container:    cont.ID,
		OutputStream: &output,
		Stream:       true,
		Stdout:       true,
	}
	waiter, err := cluster.AttachToContainerNonBlocking(attachOptions)
	if err != nil {
		return output, err
	}
	defer cluster.RemoveContainer(docker.RemoveContainerOptions{ID: cont.ID, Force: true})
	err = cluster.StartContainer(cont.ID, nil)
	if err != nil {
		return output, err
	}
	waiter.Wait()
	return output, nil
}
