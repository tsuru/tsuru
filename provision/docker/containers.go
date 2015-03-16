// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"sync"

	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
)

type appLocker struct {
	sync.Mutex
	refCount map[string]int
}

func (l *appLocker) lock(appName string) bool {
	l.Lock()
	defer l.Unlock()
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

func (l *appLocker) unlock(appName string) {
	l.Lock()
	defer l.Unlock()
	if l.refCount == nil {
		return
	}
	l.refCount[appName]--
	if l.refCount[appName] <= 0 {
		l.refCount[appName] = 0
		app.ReleaseApplicationLock(appName)
	}
}

var containerMovementErr = errors.New("Error moving some containers.")

func handleMoveErrors(moveErrors chan error, writer io.Writer) error {
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

func (p *dockerProvisioner) runReplaceUnitsPipeline(w io.Writer, a provision.App, toRemoveContainers []container, imageId string, toHosts ...string) ([]container, error) {
	var toHost string
	if len(toHosts) > 0 {
		toHost = toHosts[0]
	}
	if w == nil {
		w = ioutil.Discard
	}
	args := changeUnitsPipelineArgs{
		app:         a,
		toRemove:    toRemoveContainers,
		unitsToAdd:  len(toRemoveContainers),
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
	return pipeline.Result().([]container), nil
}

func (p *dockerProvisioner) runCreateUnitsPipeline(w io.Writer, a provision.App, toAddCount int, imageId string) ([]container, error) {
	if w == nil {
		w = ioutil.Discard
	}
	args := changeUnitsPipelineArgs{
		app:         a,
		unitsToAdd:  toAddCount,
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
	return pipeline.Result().([]container), nil
}

func (p *dockerProvisioner) moveOneContainer(c container, toHost string, errors chan error, wg *sync.WaitGroup, writer io.Writer, locker *appLocker) container {
	if wg != nil {
		defer wg.Done()
	}
	locked := locker.lock(c.AppName)
	if !locked {
		errors <- fmt.Errorf("couldn't move %s, unable to lock %q", c.ID, c.AppName)
		return container{}
	}
	defer locker.unlock(c.AppName)
	a, err := app.GetByName(c.AppName)
	if err != nil {
		errors <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("error getting app %q for unit %s", c.AppName, c.ID),
		}
		return container{}
	}
	imageId, err := appCurrentImageName(a.GetName())
	if err != nil {
		errors <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("error getting app %q image name for unit %s", c.AppName, c.ID),
		}
		return container{}
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
	addedContainers, err := p.runReplaceUnitsPipeline(nil, a, []container{c}, imageId, destHosts...)
	if err != nil {
		errors <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("Error moving unit %s", c.ID),
		}
		return container{}
	}
	prefix := "Moved unit"
	if p.isDryMode {
		prefix = "Would move unit"
	}
	fmt.Fprintf(writer, "%s %s -> %s for %q from %s -> %s\n", prefix, c.ID, addedContainers[0].ID, c.AppName, c.HostAddr, addedContainers[0].HostAddr)
	return addedContainers[0]
}

func (p *dockerProvisioner) moveContainer(contId string, toHost string, writer io.Writer) (container, error) {
	cont, err := p.getContainer(contId)
	if err != nil {
		return container{}, err
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	moveErrors := make(chan error, 1)
	locker := &appLocker{}
	createdContainer := p.moveOneContainer(*cont, toHost, moveErrors, &wg, writer, locker)
	close(moveErrors)
	return createdContainer, handleMoveErrors(moveErrors, writer)
}

func (p *dockerProvisioner) moveContainerList(containers []container, toHost string, writer io.Writer) error {
	locker := &appLocker{}
	moveErrors := make(chan error, len(containers))
	wg := sync.WaitGroup{}
	wg.Add(len(containers))
	for _, c := range containers {
		go p.moveOneContainer(c, toHost, moveErrors, &wg, writer, locker)
	}
	go func() {
		wg.Wait()
		close(moveErrors)
	}()
	return handleMoveErrors(moveErrors, writer)
}

func (p *dockerProvisioner) moveContainers(fromHost, toHost string, writer io.Writer) error {
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

type hostWithContainers struct {
	HostAddr   string `bson:"_id"`
	Count      int
	Containers []container
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
		nodes, err := p.cluster.NodesForMetadata(metadataFilter)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			hostsFilter = append(hostsFilter, urlToHost(n.Address))
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

func (p *dockerProvisioner) rebalanceContainers(writer io.Writer, dryRun bool) error {
	_, err := p.rebalanceContainersByFilter(writer, nil, nil, dryRun)
	return err
}
