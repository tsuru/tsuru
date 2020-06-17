// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
)

var _ appTypes.AppLogService = &provisionerWrapper{}

// provisionerWrapper is a layer designed to use provision native logging when is possible,
// otherwise will use backwards compatibility with own tsuru log api.
type provisionerWrapper struct {
	logService appTypes.AppLogService
}

func newProvisionerWrapper(logService appTypes.AppLogService) appTypes.AppLogService {
	return &provisionerWrapper{
		logService: logService,
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

func (k *provisionerWrapper) List(args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	a, err := servicemanager.App.GetByName(args.AppName)
	if err != nil {
		return nil, err
	}
	logsProvisioner, err := k.getLogsProvisioner(a)
	if err == provision.ErrLogsUnavailable {
		return k.logService.List(args)
	}
	if err != nil {
		return nil, err
	}

	logs, err := logsProvisioner.ListLogs(a, args)
	if err == provision.ErrLogsUnavailable {
		return k.logService.List(args)
	}
	return logs, err
}

func (k *provisionerWrapper) Watch(args appTypes.ListLogArgs) (appTypes.LogWatcher, error) {
	a, err := servicemanager.App.GetByName(args.AppName)
	if err != nil {
		return nil, err
	}
	logsProvisioner, err := k.getLogsProvisioner(a)
	if err == provision.ErrLogsUnavailable {
		return k.logService.Watch(args)
	}
	if err != nil {
		return nil, err
	}

	return logsProvisioner.WatchLogs(a, args)
}

func (k *provisionerWrapper) getLogsProvisioner(a appTypes.App) (provision.LogsProvisioner, error) {
	provisioner, err := pool.GetProvisionerForPool(a.GetPool())
	if err != nil {
		return nil, err
	}

	if logsProvisioner, ok := provisioner.(provision.LogsProvisioner); ok {
		return logsProvisioner, nil
	}

	return nil, provision.ErrLogsUnavailable
}
