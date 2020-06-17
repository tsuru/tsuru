// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	appTypes "github.com/tsuru/tsuru/types/app"
)

const (
	promNamespace = "tsuru"
	promSubsystem = "logs"
)

func AppLogService() (appTypes.AppLogService, error) {
	appLogSvc, _ := config.GetString("log:app-log-service")
	if appLogSvc == "" {
		appLogSvc = "storage"
	}
	var svc appTypes.AppLogService
	var err error
	switch appLogSvc {
	case "storage":
		svc, err = storageAppLogService()
	case "memory":
		svc, err = aggregatorAppLogService()
	default:
		return nil, errors.New(`invalid app log service, valid values are: "storage" or "memory"`)
	}
	if err != nil {
		return nil, err
	}
	return newProvisionerWrapper(svc), nil
}
