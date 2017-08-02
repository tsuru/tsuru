// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import "errors"

var (
	TeamRepository       TeamRepo
	ErrTeamAlreadyExists = errors.New("team already exists")
	ErrTeamNotFound      = errors.New("team not found")
)

type Team struct {
	Name         string `bson:"_id" json:"name"`
	CreatingUser string
}

type TeamRepo interface {
	Insert(Team) error
	FindAll() ([]Team, error)
	FindByName(string) (*Team, error)
	FindByNames([]string) ([]Team, error)
	Delete(Team) error
}
