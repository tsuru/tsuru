// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"k8s.io/client-go/util/workqueue"
)

const (
	rebuildWorkers = 20

	eventKindRebuild = "rebuild-routes-task"
)

var globalTask atomic.Value

type rebuildTask struct {
	queue     workqueue.RateLimitingInterface
	appFinder func(string) (RebuildApp, error)
	wg        sync.WaitGroup
}

func (t *rebuildTask) Shutdown(ctx context.Context) error {
	t.queue.ShutDown()
	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	return nil
}

func (t *rebuildTask) runWorkers() {
	for i := 0; i < rebuildWorkers; i++ {
		t.wg.Add(1)
		go t.runConsumer()
	}
}

func (t *rebuildTask) runConsumer() {
	defer t.wg.Done()
	for {
		shutdown := t.consumer()
		if shutdown {
			return
		}
	}
}

func (t *rebuildTask) consumer() (shutdown bool) {
	key, shutdown := t.queue.Get()
	if shutdown {
		return true
	}
	defer t.queue.Done(key)
	err := process(key)
	if err == nil {
		t.queue.Forget(key)
		return false
	}
	log.Errorf("[routes-rebuild-task] error processing app %v: %s", key, err)
	t.queue.AddRateLimited(key)
	return false
}

func process(key interface{}) error {
	appName, ok := key.(string)
	if !ok {
		return errors.Errorf("unable to convert key to appName: %#v", key)
	}
	return RunRoutesRebuildOnce(appName, true, nil)
}

func Initialize(finder func(string) (RebuildApp, error)) error {
	task := &rebuildTask{
		appFinder: finder,
		queue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"tsuru_workqueue_rebuild",
		),
	}
	task.runWorkers()
	shutdown.Register(task)
	globalTask.Store(task)
	return nil
}

func getTask() *rebuildTask {
	value := globalTask.Load()
	if value == nil {
		return nil
	}
	return value.(*rebuildTask)
}

func RunRoutesRebuildOnce(appName string, lock bool, w io.Writer) (err error) {
	task := getTask()
	if task == nil {
		return errors.New("no appFinder available")
	}
	a, err := task.appFinder(appName)
	if err != nil {
		return errors.Wrapf(err, "error getting app %q", appName)
	}
	if a == nil {
		log.Errorf("[routes-rebuild-task] app %q not found, ignoring task", appName)
		return nil
	}
	var result map[string]RebuildRoutesResult
	if lock {
		var evt *event.Event
		evt, err = event.NewInternal(&event.Opts{
			Target:       event.Target{Type: event.TargetTypeApp, Value: appName},
			InternalKind: eventKindRebuild,
			Allowed:      event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, appName)),
		})
		if err != nil {
			if lockedErr, ok := err.(event.ErrEventLocked); ok {
				lockedEvt := lockedErr.Event
				if lockedEvt.Kind.Name == eventKindRebuild {
					log.Debugf("[routes-rebuild-task] app %q already locked in rebuild, ignoring task", appName)
					return nil
				}
			}
			return errors.Errorf("unable to create rebuild routes event %q: %v", appName, err)
		}

		defer func() {
			if err != nil {
				evt.DoneCustomData(err, result)
				return
			}
			evt.Abort()
		}()
	}
	result, err = RebuildRoutes(context.TODO(), RebuildRoutesOpts{
		App:    a,
		Writer: w,
	})

	if err != nil {
		return errors.Wrapf(err, "error rebuilding app %q", appName)
	}
	return nil
}

func RoutesRebuildOrEnqueue(appName string) {
	routesRebuildOrEnqueueOptionalLock(appName, false, nil)
}

func RoutesRebuildOrEnqueueWithProgress(appName string, w io.Writer) {
	routesRebuildOrEnqueueOptionalLock(appName, false, w)
}

func LockedRoutesRebuildOrEnqueue(appName string) {
	routesRebuildOrEnqueueOptionalLock(appName, true, nil)
}

func EnqueueRoutesRebuild(appName string) {
	task := getTask()
	if task != nil {
		task.queue.Add(appName)
	}
}

func routesRebuildOrEnqueueOptionalLock(appName string, lock bool, w io.Writer) {
	err := RunRoutesRebuildOnce(appName, lock, w)
	if err == nil {
		return
	}
	log.Errorf("[routes-rebuild-task] error running rebuild, enqueueing task: %v", err)
	EnqueueRoutesRebuild(appName)
}

func Shutdown(ctx context.Context) error {
	task := getTask()
	if task != nil {
		return task.Shutdown(ctx)
	}
	return nil
}
