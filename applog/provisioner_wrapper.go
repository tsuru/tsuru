// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	logTypes "github.com/tsuru/tsuru/types/log"
)

var (
	_ appTypes.AppLogService         = &provisionerWrapper{}
	_ appTypes.AppLogServiceInstance = &provisionerWrapper{}
)

// provisionerWrapper is a layer designed to use provision native logging when is possible,
// otherwise will use backwards compatibility with own tsuru log api.
type provisionerWrapper struct {
	logService        appTypes.AppLogService
	provisionerGetter logsProvisionerGetter
}

func newProvisionerWrapper(logService appTypes.AppLogService) appTypes.AppLogService {
	return &provisionerWrapper{
		logService:        logService,
		provisionerGetter: defaultLogsProvisionerGetter,
	}
}

// Add is uncalled when the target pool uses own provisioner log stack
func (k *provisionerWrapper) Add(appName, message, source, unit string) error {
	return k.logService.Add(appName, message, source, unit)
}

// Enqueue is uncalled when the target pool uses own provisioner log stack
func (k *provisionerWrapper) Enqueue(entry *appTypes.Applog) error {
	return k.logService.Enqueue(entry)
}

func defineLogabbleObject(ctx context.Context, lType logTypes.LogType, name string) (logTypes.LogabbleObject, error) {
	if lType == logTypes.LogTypeJob {
		return servicemanager.Job.GetByName(ctx, name)
	}
	return servicemanager.App.GetByName(ctx, name)
}

func (k *provisionerWrapper) List(ctx context.Context, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	logs := []appTypes.Applog{}
	obj, err := defineLogabbleObject(ctx, args.Type, args.Name)
	if err != nil {
		return nil, err
	}
	if args.Type == logTypes.LogTypeApp || args.Type == "" {
		logs, err = k.logService.List(ctx, args)
		if err != nil {
			return nil, err
		}
	}
	logsProvisioner, err := k.provisionerGetter(ctx, obj)
	if err == provision.ErrLogsUnavailable {
		return logs, nil
	}
	if err != nil {
		return nil, err
	}
	provLogs, err := logsProvisioner.ListLogs(ctx, obj, args)
	if err == provision.ErrLogsUnavailable {
		return logs, nil
	}
	if err != nil {
		return nil, err
	}
	logs = append(logs, provLogs...)
	sort.SliceStable(logs, func(i, j int) bool {
		return logs[i].Date.Before(logs[j].Date)
	})
	return logs, err
}

func (k *provisionerWrapper) Watch(ctx context.Context, args appTypes.ListLogArgs) (appTypes.LogWatcher, error) {
	var tsuruWatcher appTypes.LogWatcher
	obj, err := defineLogabbleObject(ctx, args.Type, args.Name)
	if err != nil {
		return nil, err
	}
	if args.Type == logTypes.LogTypeApp || args.Type == "" {
		tsuruWatcher, err = k.logService.Watch(ctx, args)
		if err != nil {
			return nil, err
		}
	}
	logsProvisioner, err := k.provisionerGetter(ctx, obj)
	if err == provision.ErrLogsUnavailable && args.Type == logTypes.LogTypeApp {
		return tsuruWatcher, nil
	}
	if err != nil {
		return nil, err
	}
	provisionerWatcher, err := logsProvisioner.WatchLogs(ctx, obj, args)
	if err == provision.ErrLogsUnavailable && tsuruWatcher != nil {
		return tsuruWatcher, nil
	}
	if err != nil {
		return nil, err
	}
	if tsuruWatcher != nil {
		return newMultiWatcher(provisionerWatcher, tsuruWatcher), nil
	}
	return newMultiWatcher(provisionerWatcher), nil
}

func (k *provisionerWrapper) Instance() appTypes.AppLogService {
	if svcInstance, ok := k.logService.(appTypes.AppLogServiceInstance); ok {
		return svcInstance.Instance()
	}

	return k.logService
}

type logsProvisionerGetter func(ctx context.Context, obj logTypes.LogabbleObject) (provision.LogsProvisioner, error)

var defaultLogsProvisionerGetter = func(ctx context.Context, obj logTypes.LogabbleObject) (provision.LogsProvisioner, error) {
	provisioner, err := pool.GetProvisionerForPool(ctx, obj.GetPool())
	if err != nil {
		return nil, err
	}

	if logsProvisioner, ok := provisioner.(provision.LogsProvisioner); ok {
		return logsProvisioner, nil
	}

	return nil, provision.ErrLogsUnavailable
}

var _ appTypes.LogWatcher = &multiWatcher{}

type multiWatcher struct {
	subWatchers []appTypes.LogWatcher
	ch          chan appTypes.Applog
	close       chan struct{}
	closeCalled int32
	wg          sync.WaitGroup
}

func newMultiWatcher(subWatchers ...appTypes.LogWatcher) *multiWatcher {
	watcher := &multiWatcher{
		subWatchers: subWatchers,
		ch:          make(chan appTypes.Applog, 1000),
		close:       make(chan struct{}),
	}

	watcher.wg.Add(len(subWatchers))
	for _, subWatcher := range subWatchers {
		go watcher.startConsume(subWatcher)
	}

	return watcher
}

func (m *multiWatcher) startConsume(subWatcher appTypes.LogWatcher) {
	defer m.wg.Done()
	c := subWatcher.Chan()
	for {
		select {
		case log, open := <-c:

			if !open {
				return
			}

			select {
			case m.ch <- log:
			case <-m.close:
				return

			}
		case <-m.close:
			return
		}
	}
}
func (m *multiWatcher) Chan() <-chan appTypes.Applog {
	return m.ch
}
func (m *multiWatcher) Close() {
	if atomic.AddInt32(&m.closeCalled, 1) != 1 {
		return
	}

	close(m.close)
	for _, subWatcher := range m.subWatchers {
		subWatcher.Close()
	}
	m.wg.Wait()
	close(m.ch)
}
