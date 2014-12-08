// Copyright 2014 tsuru authors. All rights reserved.
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

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2/bson"
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

func runReplaceUnitsPipeline(w io.Writer, a provision.App, toRemoveContainers []container, toHosts ...string) ([]container, error) {
	var toHost string
	if len(toHosts) > 0 {
		toHost = toHosts[0]
	}
	if w == nil {
		w = ioutil.Discard
	}
	args := changeUnitsPipelineArgs{
		app:        a,
		toRemove:   toRemoveContainers,
		unitsToAdd: len(toRemoveContainers),
		toHost:     toHost,
		writer:     w,
	}
	pipeline := action.NewPipeline(
		&provisionAddUnitsToHost,
		&addNewRoutes,
		&removeOldRoutes,
		&provisionRemoveOldUnits,
	)
	err := pipeline.Execute(args)
	if err != nil {
		return nil, err
	}
	err = removeImage(assembleImageName(a.GetName(), a.GetPlatform()))
	if err != nil {
		log.Debugf("Ignored error removing old images: %s", err.Error())
	}
	return pipeline.Result().([]container), nil
}

func runCreateUnitsPipeline(w io.Writer, a provision.App, toAddCount int) ([]container, error) {
	if w == nil {
		w = ioutil.Discard
	}
	args := changeUnitsPipelineArgs{
		app:        a,
		unitsToAdd: toAddCount,
		writer:     w,
	}
	pipeline := action.NewPipeline(
		&provisionAddUnitsToHost,
		&addNewRoutes,
	)
	err := pipeline.Execute(args)
	if err != nil {
		return nil, err
	}
	return pipeline.Result().([]container), nil
}

func moveOneContainer(c container, toHost string, errors chan error, wg *sync.WaitGroup, encoder *json.Encoder, locker *appLocker) container {
	if wg != nil {
		defer wg.Done()
	}
	locked := locker.lock(c.AppName)
	if !locked {
		errors <- fmt.Errorf("Couldn't move %s, unable to lock %q.", c.ID, c.AppName)
		return container{}
	}
	defer locker.unlock(c.AppName)
	a, err := app.GetByName(c.AppName)
	if err != nil {
		errors <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("Error getting app %q for unit %s.", c.AppName, c.ID),
		}
		return container{}
	}
	logProgress(encoder, "Moving unit %s for %q: %s -> %s...", c.ID, c.AppName, c.HostAddr, toHost)
	addedContainers, err := runReplaceUnitsPipeline(nil, a, []container{c}, toHost)
	if err != nil {
		errors <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("Error moving unit %s.", c.ID),
		}
		return container{}
	}
	logProgress(encoder, "Finished moving unit %s for %q.", c.ID, c.AppName)
	addedUnit := addedContainers[0].asUnit(a)
	err = a.BindUnit(&addedUnit)
	if err != nil {
		errors <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("Error binding unit %s to service instances.", c.ID),
		}
		return container{}
	}
	logProgress(encoder, "Moved unit %s -> %s for %s with bindings.", c.ID, addedUnit.Name, c.AppName)
	return addedContainers[0]
}

func moveContainer(contId string, toHost string, encoder *json.Encoder) (container, error) {
	cont, err := getContainer(contId)
	if err != nil {
		return container{}, err
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	moveErrors := make(chan error, 1)
	locker := &appLocker{}
	createdContainer := moveOneContainer(*cont, toHost, moveErrors, &wg, encoder, locker)
	close(moveErrors)
	return createdContainer, handleMoveErrors(moveErrors, encoder)
}

func moveContainers(fromHost, toHost string, encoder *json.Encoder) error {
	containers, err := listContainersByHost(fromHost)
	if err != nil {
		return err
	}
	numberContainers := len(containers)
	if numberContainers == 0 {
		logProgress(encoder, "No units to move in %s.", fromHost)
		return nil
	}
	logProgress(encoder, "Moving %d units...", numberContainers)
	locker := &appLocker{}
	moveErrors := make(chan error, numberContainers)
	wg := sync.WaitGroup{}
	wg.Add(numberContainers)
	for _, c := range containers {
		go moveOneContainer(c, toHost, moveErrors, &wg, encoder, locker)
	}
	go func() {
		wg.Wait()
		close(moveErrors)
	}()
	return handleMoveErrors(moveErrors, encoder)
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

func rebalanceContainers(encoder *json.Encoder, dryRun bool) error {
	coll := collection()
	defer coll.Close()
	fullDocQuery := bson.M{
		// Could use $$ROOT instead of repeating fields but only in Mongo 2.6+.
		"_id":      "$_id",
		"id":       "$id",
		"name":     "$name",
		"appname":  "$appname",
		"type":     "$type",
		"ip":       "$ip",
		"image":    "$image",
		"hostaddr": "$hostaddr",
		"hostport": "$hostport",
		"status":   "$status",
		"version":  "$version",
	}
	appsPipe := coll.Pipe([]bson.M{
		{"$group": bson.M{"_id": "$appname", "count": bson.M{"$sum": 1}}},
	})
	var appsInfo []struct {
		Name  string `bson:"_id"`
		Count int
	}
	err := appsPipe.All(&appsInfo)
	if err != nil {
		return err
	}
	clusterInstance := dockerCluster()
	for _, appInfo := range appsInfo {
		if appInfo.Count < 2 {
			continue
		}
		logProgress(encoder, "Rebalancing app %q (%d units)...", appInfo.Name, appInfo.Count)
		var possibleDests []cluster.Node
		if isSegregateScheduler() {
			possibleDests, err = nodesForAppName(clusterInstance, appInfo.Name)
		} else {
			possibleDests, err = clusterInstance.Nodes()
		}
		if err != nil {
			return err
		}
		maxContPerUnit := appInfo.Count / len(possibleDests)
		overflowHosts := appInfo.Count % len(possibleDests)
		pipe := coll.Pipe([]bson.M{
			{"$match": bson.M{"hostaddr": bson.M{"$ne": ""}, "appname": appInfo.Name}},
			{"$group": bson.M{
				"_id":        "$hostaddr",
				"count":      bson.M{"$sum": 1},
				"containers": bson.M{"$push": fullDocQuery}}},
		})
		var hosts []hostWithContainers
		hostsSet := make(map[string]bool)
		err = pipe.All(&hosts)
		if err != nil {
			return err
		}
		for _, host := range hosts {
			hostsSet[host.HostAddr] = true
		}
		for _, node := range possibleDests {
			hostAddr := urlToHost(node.Address)
			_, present := hostsSet[hostAddr]
			if !present {
				hosts = append(hosts, hostWithContainers{HostAddr: hostAddr})
			}
		}
		anythingDone := false
		for _, host := range hosts {
			toMoveCount := host.Count - maxContPerUnit
			if toMoveCount <= 0 {
				continue
			}
			if overflowHosts > 0 {
				overflowHosts--
				toMoveCount--
				if toMoveCount <= 0 {
					continue
				}
			}
			anythingDone = true
			logProgress(encoder, "Trying to move %d units for %q from %s...", toMoveCount, appInfo.Name, host.HostAddr)
			locker := &appLocker{}
			wg := sync.WaitGroup{}
			moveErrors := make(chan error, toMoveCount)
			for _, cont := range host.Containers {
				minDest := minCountHost(hosts)
				if minDest.Count < maxContPerUnit {
					toMoveCount--
					minDest.Count++
					if dryRun {
						logProgress(encoder, "Would move unit %s for %q: %s -> %s...", cont.ID, appInfo.Name, cont.HostAddr, minDest.HostAddr)
					} else {
						wg.Add(1)
						go moveOneContainer(cont, minDest.HostAddr, moveErrors, &wg, encoder, locker)
					}
				}
				if toMoveCount == 0 {
					break
				}
			}
			if toMoveCount > 0 {
				logProgress(encoder, "Couldn't find suitable destination for %d units for app %q", toMoveCount, appInfo.Name)
			}
			go func() {
				wg.Wait()
				close(moveErrors)
			}()
			err := handleMoveErrors(moveErrors, encoder)
			if err != nil {
				return err
			}
		}
		if anythingDone {
			logProgress(encoder, "Rebalance finished for %q", appInfo.Name)
		} else {
			logProgress(encoder, "Nothing to do for %q", appInfo.Name)
		}
	}
	return nil
}
