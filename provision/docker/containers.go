// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"fmt"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"labix.org/v2/mgo/bson"
	"math"
	"sync"
)

var appDBMutex sync.Mutex

type progressLog struct {
	Message string
}

func logProgress(encoder *json.Encoder, format string, params ...interface{}) {
	encoder.Encode(progressLog{Message: fmt.Sprintf(format, params...)})
}

func moveOneContainerInDB(a *app.App, oldContainer container, newUnit provision.Unit) error {
	appDBMutex.Lock()
	defer appDBMutex.Unlock()
	err := a.AddUnitsToDB([]provision.Unit{newUnit})
	if err != nil {
		return err
	}
	return a.RemoveUnitFromDB(oldContainer.ID)
}

func moveOneContainer(c container, toHost string, errors chan error, wg *sync.WaitGroup, encoder *json.Encoder) {
	a, err := app.GetByName(c.AppName)
	defer wg.Done()
	if err != nil {
		errors <- err
		return
	}
	logProgress(encoder, "Moving unit %s for %q: %s -> %s...", c.ID, c.AppName, c.HostAddr, toHost)
	pipeline := action.NewPipeline(
		&provisionAddUnitToHost,
		&provisionRemoveOldUnit,
	)
	err = pipeline.Execute(a, toHost, c)
	if err != nil {
		errors <- err
		return
	}
	logProgress(encoder, "Finished moving unit %s for %q.", c.ID, c.AppName)
	addedUnit := pipeline.Result().(provision.Unit)
	err = moveOneContainerInDB(a, c, addedUnit)
	if err != nil {
		errors <- err
		return
	}
	logProgress(encoder, "Moved unit %s -> %s for %s in DB.", c.ID, addedUnit.Name, c.AppName)
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
	moveErrors := make(chan error, numberContainers)
	wg := sync.WaitGroup{}
	wg.Add(numberContainers)
	for _, c := range containers {
		go moveOneContainer(c, toHost, moveErrors, &wg, encoder)
	}
	go func() {
		wg.Wait()
		close(moveErrors)
	}()
	var lastError error = nil
	for err := range moveErrors {
		log.Errorf("Error moving container - %s", err)
		lastError = err
	}
	return lastError
}

type hostWithContainers struct {
	HostAddr   string `bson:"_id"`
	Count      int
	Containers []container
}

func minHost(hosts map[string]*hostWithContainers, possibleDests []cluster.Node) *hostWithContainers {
	var minHost *hostWithContainers
	minCount := math.MaxInt32
	for _, dest := range possibleDests {
		hostAddr := urlToHost(dest.Address)
		host := hosts[hostAddr]
		if host.Count < minCount {
			minCount = host.Count
			minHost = host
		}
	}
	return minHost
}

func rebalanceContainers(encoder *json.Encoder, dryRun bool) error {
	coll := collection()
	defer coll.Close()
	pipe := coll.Pipe([]bson.M{
		{"$match": bson.M{"hostaddr": bson.M{"$ne": ""}}},
		{"$group": bson.M{
			"_id":   "$hostaddr",
			"count": bson.M{"$sum": 1},
			"containers": bson.M{"$push": bson.M{
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
			}}}},
	})
	var hosts []hostWithContainers
	hostsMap := make(map[string]*hostWithContainers)
	err := pipe.All(&hosts)
	if err != nil {
		return err
	}
	totalCount := 0
	for i, host := range hosts {
		hostsMap[host.HostAddr] = &hosts[i]
		totalCount += host.Count
	}
	cluster := dockerCluster()
	allNodes, err := cluster.Nodes()
	if err != nil {
		return err
	}
	for _, node := range allNodes {
		hostAddr := urlToHost(node.Address)
		_, present := hostsMap[hostAddr]
		if !present {
			hosts = append(hosts, hostWithContainers{HostAddr: hostAddr})
			hostsMap[hostAddr] = &hosts[len(hosts)-1]
		}
	}
	numberOfNodes := len(allNodes)
	maxContsPerUnit := int(math.Ceil(float64(totalCount) / float64(numberOfNodes)))
	for _, host := range hosts {
		toMoveCount := host.Count - maxContsPerUnit
		if toMoveCount <= 0 {
			continue
		}
		logProgress(encoder, "Trying to move %d units from %s...", toMoveCount, host.HostAddr)
		wg := sync.WaitGroup{}
		moveErrors := make(chan error, toMoveCount)
		for _, cont := range host.Containers {
			possibleDests, err := cluster.NodesForOptions(cont.AppName)
			if err != nil {
				return err
			}
			minDest := minHost(hostsMap, possibleDests)
			if minDest.Count < maxContsPerUnit {
				toMoveCount--
				minDest.Count++
				if dryRun {
					logProgress(encoder, "Would move unit %s for %q: %s -> %s...", cont.ID, cont.AppName, cont.HostAddr, minDest.HostAddr)
				} else {
					wg.Add(1)
					go moveOneContainer(cont, minDest.HostAddr, moveErrors, &wg, encoder)
				}
			}
			if toMoveCount == 0 {
				break
			}
		}
		go func() {
			wg.Wait()
			close(moveErrors)
		}()
		for err := range moveErrors {
			log.Errorf("Error moving container - %s", err)
			return err
		}
	}
	return nil
}
