// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"errors"

	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/types/quota"
)

var _ quota.QuotaItem = &Team{}

// Team represents a real world team, a team has one creating user and a name.
type Team struct {
	Name         string      `json:"name"`
	CreatingUser string      `json:"creatingUser"`
	Tags         []string    `json:"tags"`
	Quota        quota.Quota `json:"quota"`
}

func (t Team) GetName() string {
	return t.Name
}

type TeamService interface {
	Create(context.Context, string, []string, *User) error
	Update(context.Context, string, []string) error
	List(context.Context) ([]Team, error)
	FindByName(context.Context, string) (*Team, error)
	FindByNames(context.Context, []string) ([]Team, error)
	Remove(context.Context, string) error
}

type TeamStorage interface {
	Insert(context.Context, Team) error
	Update(context.Context, Team) error
	FindAll(context.Context) ([]Team, error)
	FindByName(context.Context, string) (*Team, error)
	FindByNames(context.Context, []string) ([]Team, error)
	Delete(context.Context, Team) error
}

var (
	ErrInvalidTeamName = &tsuruErrors.ValidationError{
		Message: "Invalid team name, team names should start with a letter and" +
			"contain only lower case letters, numbers, dashes, underscore and @.",
	}
	ErrTeamAlreadyExists = errors.New("team already exists")
	ErrTeamNotFound      = errors.New("team not found")
)
