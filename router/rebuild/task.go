// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild

import (
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/queue"
)

const routesRebuildTaskName = "rebuildRoutesTask"

var routesRebuildRetryTime = 10 * time.Second

var appFinder func(string) (RebuildApp, error)

type routesRebuildTask struct{}

func (t *routesRebuildTask) Name() string {
	return routesRebuildTaskName
}

func (t *routesRebuildTask) Run(job monsterqueue.Job) {
	params := job.Parameters()
	appName, ok := params["appName"].(string)
	if !ok {
		job.Error(errors.New("invalid parameters, expected appName"))
	}
	for !runRoutesRebuildOnce(appName, true) {
		time.Sleep(routesRebuildRetryTime)
	}
	job.Success(nil)
}

func RegisterTask(finder func(string) (RebuildApp, error)) error {
	appFinder = finder
	q, err := queue.Queue()
	if err != nil {
		return err
	}
	return q.RegisterTask(&routesRebuildTask{})
}

func runRoutesRebuildOnce(appName string, lock bool) bool {
	if appFinder == nil {
		return false
	}
	a, err := appFinder(appName)
	if err != nil {
		log.Errorf("[routes-rebuild-task] error getting app %q: %s", appName, err)
		return false
	}
	if a == nil {
		log.Errorf("[routes-rebuild-task] app %q not found, aborting", appName)
		return true
	}
	if lock {
		var locked bool
		locked, err = a.InternalLock("rebuild-routes-task")
		if err != nil || !locked {
			return false
		}
		defer a.Unlock()
	}
	_, err = rebuildRoutesAsync(a, false)
	if err != nil {
		log.Errorf("[routes-rebuild-task] error rebuilding app %q: %s", appName, err)
		return false
	}
	return true
}

func RoutesRebuildOrEnqueue(appName string) {
	routesRebuildOrEnqueueOptionalLock(appName, false)
}

func LockedRoutesRebuildOrEnqueue(appName string) {
	routesRebuildOrEnqueueOptionalLock(appName, true)
}

func routesRebuildOrEnqueueOptionalLock(appName string, lock bool) {
	if runRoutesRebuildOnce(appName, lock) {
		return
	}
	q, err := queue.Queue()
	if err != nil {
		log.Errorf("unable to enqueue rebuild routes task: %s", err)
		return
	}
	_, err = q.Enqueue(routesRebuildTaskName, monsterqueue.JobParams{
		"appName": appName,
	})
	if err != nil {
		log.Errorf("unable to enqueue rebuild routes task: %s", err)
		return
	}
}
