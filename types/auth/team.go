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
	Tags         []string `json:"tags"`
}

type TeamService interface {
	Create(string, []string, *User) error
	List() ([]Team, error)
	FindByName(string) (*Team, error)
	FindByNames([]string) ([]Team, error)
	Remove(string) error
}

type TeamStorage interface {
	Insert(Team) error
	FindAll() ([]Team, error)
	FindByName(string) (*Team, error)
	FindByNames([]string) ([]Team, error)
	Delete(Team) error
}

var (
	ErrInvalidTeamName = &tsuruErrors.ValidationError{
		Message: "Invalid team name, team names should start with a letter and" +
			"contain only lower case letters, numbers, dashes, underscore and @.",
	}
	ErrTeamAlreadyExists = errors.New("team already exists")
	ErrTeamNotFound      = errors.New("team not found")
)
