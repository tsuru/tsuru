// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"github.com/pkg/errors"
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
