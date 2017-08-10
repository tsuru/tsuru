// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/types/auth"
)

type DbDriver struct {
	TeamService auth.TeamService
}

var DbDrivers = make(map[string]DbDriver)

func RegisterDbDriver(name string, driver DbDriver) {
	DbDrivers[name] = driver
}

func UnregisterDbDriver(name string) {
	delete(DbDrivers, name)
}

func GetDbDriver(name string) (*DbDriver, error) {
	driver, ok := DbDrivers[name]
	if !ok {
		return nil, errors.Errorf("Unknown database driver: %q.", name)
	}
	return &driver, nil
}

func GetCurrentDbDriver() (*DbDriver, error) {
	driverLock.RLock()
	if teamService != nil {
		driverLock.RUnlock()
		return teamService, nil
	}
	driverLock.RUnlock()
	driverLock.Lock()
	defer driverLock.Unlock()
	if teamService != nil {
		return teamService, nil
	}
	dbDriverName, err := config.GetString("database:driver")
	if err != nil {
		return nil, err
	}
	if dbDriverName == "" {
		dbDriverName = "mongodb"
	}
	dbDriver, err := storage.GetDbDriver(dbDriverName)
	if err != nil {
		return nil, err
	}
	teamService = &dbDriver.TeamService
	return teamService, nil
}
