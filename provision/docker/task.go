// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"time"

	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/queue"
)

const routesRebuildTaskName = "rebuildRoutesTask"

var routesRebuildRetryTime = 10 * time.Second

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

func registerRoutesRebuildTask() error {
	q, err := queue.Queue()
	if err != nil {
		return err
	}
	return q.RegisterTask(&routesRebuildTask{})
}

func runRoutesRebuildOnce(appName string, lock bool) bool {
	if lock {
		locked, err := app.AcquireApplicationLock(appName, app.InternalAppName, "rebuild-routes-task")
		if err != nil || !locked {
			return false
		}
		defer app.ReleaseApplicationLock(appName)
	}
	a, err := app.GetByName(appName)
	if err == app.ErrAppNotFound {
		return true
	}
	if err != nil {
		log.Errorf("[routes-rebuild-task] error getting app: %s", err)
		return false
	}
	_, err = a.RebuildRoutes()
	if err != nil {
		log.Errorf("[routes-rebuild-task] error rebuilding: %s", err)
		return false
	}
	return true
}

func routesRebuildOrEnqueue(appName string) {
	routesRebuildOrEnqueueOptionalLock(appName, false)
}

func lockedRoutesRebuildOrEnqueue(appName string) {
	routesRebuildOrEnqueueOptionalLock(appName, true)
}

func routesRebuildOrEnqueueOptionalLock(appName string, lock bool) {
	if runRoutesRebuildOnce(appName, lock) {
		return
	}
	q, err := queue.Queue()
	if err != nil {
		log.Errorf("unable to enqueue rebuild routes task: %s", err.Error())
		return
	}
	_, err = q.Enqueue(routesRebuildTaskName, monsterqueue.JobParams{
		"appName": appName,
	})
	if err != nil {
		log.Errorf("unable to enqueue rebuild routes task: %s", err.Error())
		return
	}
}
