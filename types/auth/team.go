// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"

	tsuruErrors "github.com/tsuru/tsuru/errors"
)

// Team represents a real world team, a team has one creating user and a name.
type Team struct {
	Name         string `json:"name"`
	CreatingUser string
}

type TeamService interface {
	Insert(Team) error
	FindAll() ([]Team, error)
	FindByName(string) (*Team, error)
	FindByNames([]string) ([]Team, error)
	Delete(Team) error
}

var (
	ErrInvalidTeamName = &tsuruErrors.ValidationError{
		Message: "Invalid team name, team name should have at most 63 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter.",
	}
	ErrTeamAlreadyExists = errors.New("team already exists")
	ErrTeamNotFound      = errors.New("team not found")
)
