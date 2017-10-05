// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/types/auth"
)

type TeamService struct {
	storage storage.TeamStorage
}

var ts auth.TeamService

func Team() auth.TeamService {
	if ts == nil {
		ts = &TeamService{
			storage: teamStorage(),
		}
	}
	return ts
}

func teamStorage() storage.TeamStorage {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil
		}
	}
	return dbDriver.TeamStorage
}

func (t *TeamService) Insert(team auth.Team) error {
	return t.storage.Insert(team)
}

func (t *TeamService) FindAll() ([]auth.Team, error) {
	return t.storage.FindAll()
}

func (t *TeamService) FindByName(name string) (*auth.Team, error) {
	return t.storage.FindByName(name)
}

func (t *TeamService) FindByNames(names []string) ([]auth.Team, error) {
	return t.storage.FindByNames(names)
}

func (t *TeamService) Delete(team auth.Team) error {
	return t.storage.Delete(team)
}
