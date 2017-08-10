// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/types/auth"
)

type DbDriver struct {
	TeamService auth.TeamService
}

var (
	DefaultDbDriverName = "mongodb"
	DbDrivers           = make(map[string]DbDriver)
	driverLock          sync.RWMutex
	currentDbDriver     *DbDriver
)

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
	if currentDbDriver != nil {
		driverLock.RUnlock()
		return currentDbDriver, nil
	}
	driverLock.RUnlock()
	driverLock.Lock()
	defer driverLock.Unlock()
	if currentDbDriver != nil {
		return currentDbDriver, nil
	}
	dbDriverName, err := config.GetString("database:driver")
	if err != nil || dbDriverName == "" {
		dbDriverName = DefaultDbDriverName
	}
	currentDbDriver, err = GetDbDriver(dbDriverName)
	if err != nil {
		return nil, err
	}
	return currentDbDriver, nil
}
