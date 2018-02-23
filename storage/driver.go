// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/cache"
)

type DbDriver struct {
	TeamStorage     TeamStorage
	PlatformService app.PlatformService
	PlanService     app.PlanService
	CacheService    cache.CacheService
}

var (
	DefaultDbDriverName = "mongodb"
	dbDrivers           = make(map[string]DbDriver)
	driverLock          sync.RWMutex
	currentDbDriver     *DbDriver
)

// RegisterDbDriver registers a new DB driver
func RegisterDbDriver(name string, driver DbDriver) {
	dbDrivers[name] = driver
}

// GetDbDriver returns the DB driver that was registered with a specific name
func GetDbDriver(name string) (*DbDriver, error) {
	driver, ok := dbDrivers[name]
	if !ok {
		return nil, errors.Errorf("Unknown database driver: %q.", name)
	}
	return &driver, nil
}

// GetCurrentDbDriver returns the DB driver specified in the configuration file.
// If this configuration was omitted, it returns the default DB driver
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

// GetDefaultDbDriver returns the default DB driver
func GetDefaultDbDriver() (*DbDriver, error) {
	return GetDbDriver(DefaultDbDriverName)
}
