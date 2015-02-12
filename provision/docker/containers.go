// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
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

var (
	appDBMutex sync.Mutex
	logMutex   sync.Mutex
)

type progressLog struct {
	Message string
}

type appLocker struct {
	refCount map[string]int
}

func (l *appLocker) lock(appName string) bool {
	appDBMutex.Lock()
	defer appDBMutex.Unlock()
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
	appDBMutex.Lock()
	defer appDBMutex.Unlock()
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

func logProgress(encoder *json.Encoder, format string, params ...interface{}) {
	logMutex.Lock()
	defer logMutex.Unlock()
	encoder.Encode(progressLog{Message: fmt.Sprintf(format, params...)})
}

func handleMoveErrors(moveErrors chan error, encoder *json.Encoder) error {
	hasError := false
	for err := range moveErrors {
		errMsg := fmt.Sprintf("Error moving container: %s", err.Error())
		log.Error(errMsg)
		logProgress(encoder, "%s", errMsg)
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
	if p.dryMode {
		pipeline = action.NewPipeline(
			&provisionAddUnitsToHost,
			&provisionRemoveOldUnits,
		)
	} else {
		pipeline = action.NewPipeline(
			&provisionAddUnitsToHost,
			&addNewRoutes,
			&removeOldRoutes,
			&provisionRemoveOldUnits,
			&provisionUnbindOldUnits,
			&updateAppImage,
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
		&addNewRoutes,
		&updateAppImage,
	)
	err := pipeline.Execute(args)
	if err != nil {
		return nil, err
	}
	return pipeline.Result().([]container), nil
}

func (p *dockerProvisioner) moveOneContainer(c container, toHost string, errors chan error, wg *sync.WaitGroup, encoder *json.Encoder, locker *appLocker) container {
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
	if !p.dryMode {
		logProgress(encoder, "Moving unit %s for %q from %s%s...", c.ID, c.AppName, c.HostAddr, suffix)
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
	if p.dryMode {
		prefix = "Would move unit"
	}
	logProgress(encoder, "%s %s -> %s for %q from %s -> %s", prefix, c.ID, addedContainers[0].ID, c.AppName, c.HostAddr, addedContainers[0].HostAddr)
	return addedContainers[0]
}

func (p *dockerProvisioner) moveContainer(contId string, toHost string, encoder *json.Encoder) (container, error) {
	cont, err := p.getContainer(contId)
	if err != nil {
		return container{}, err
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	moveErrors := make(chan error, 1)
	locker := &appLocker{}
	createdContainer := p.moveOneContainer(*cont, toHost, moveErrors, &wg, encoder, locker)
	close(moveErrors)
	return createdContainer, handleMoveErrors(moveErrors, encoder)
}

func (p *dockerProvisioner) moveContainerList(containers []container, toHost string, encoder *json.Encoder) error {
	locker := &appLocker{}
	moveErrors := make(chan error, len(containers))
	wg := sync.WaitGroup{}
	wg.Add(len(containers))
	for _, c := range containers {
		go p.moveOneContainer(c, toHost, moveErrors, &wg, encoder, locker)
	}
	go func() {
		wg.Wait()
		close(moveErrors)
	}()
	return handleMoveErrors(moveErrors, encoder)
}

func (p *dockerProvisioner) moveContainers(fromHost, toHost string, encoder *json.Encoder) error {
	containers, err := p.listContainersByHost(fromHost)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		logProgress(encoder, "No units to move in %s", fromHost)
		return nil
	}
	logProgress(encoder, "Moving %d units...", len(containers))
	return p.moveContainerList(containers, toHost, encoder)
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

func (p *dockerProvisioner) rebalanceContainersByFilter(encoder *json.Encoder, appFilter []string, metadataFilter map[string]string, dryRun bool) error {
	var hostsFilter []string
	if metadataFilter != nil {
		nodes, err := p.cluster.NodesForMetadata(metadataFilter)
		if err != nil {
			return err
		}
		for _, n := range nodes {
			hostsFilter = append(hostsFilter, urlToHost(n.Address))
		}
	}
	containers, err := p.listContainersByAppAndHost(appFilter, hostsFilter)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		logProgress(encoder, "No containers found to rebalance")
		return nil
	}
	if dryRun {
		p, err = p.DryMode(containers)
		if err != nil {
			return err
		}
		defer p.StopDryMode()
	}
	logProgress(encoder, "Rebalancing %d units...", len(containers))
	return p.moveContainerList(containers, "", encoder)
}

func (p *dockerProvisioner) rebalanceContainers(encoder *json.Encoder, dryRun bool) error {
	return p.rebalanceContainersByFilter(encoder, nil, nil, dryRun)
}
