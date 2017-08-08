// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"errors"

	"github.com/tsuru/tsuru/types/auth"
)

var (
	TeamRepository       TeamRepo
	ErrTeamAlreadyExists = errors.New("team already exists")
	ErrTeamNotFound      = errors.New("team not found")
)

type TeamRepo interface {
	Insert(auth.Team) error
	FindAll() ([]auth.Team, error)
	FindByName(string) (*auth.Team, error)
	FindByNames([]string) ([]auth.Team, error)
	Delete(auth.Team) error
}
