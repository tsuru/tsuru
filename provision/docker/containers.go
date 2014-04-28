// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"fmt"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
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
