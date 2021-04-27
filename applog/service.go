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
)

func AppLogService() (appTypes.AppLogService, error) {
	appLogSvc, _ := config.GetString("log:app-log-service")
	if appLogSvc == "" {
		appLogSvc = "memory-standalone"
	}
	var svc appTypes.AppLogService
	var err error
	switch appLogSvc {
	case "memory-standalone":
		svc, err = memoryAppLogService()
	case "memory":
		svc, err = aggregatorAppLogService()
	default:
		return nil, errors.New(`invalid app log service, valid values are: "memory" or "memory-standalone"`)
	}
	if err != nil {
		return nil, err
	}
	return newProvisionerWrapper(svc), nil
}
