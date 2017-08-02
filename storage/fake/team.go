// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fake

import "github.com/tsuru/tsuru/storage"

type TeamRepository struct {
	teams map[string]storage.Team
}

func init() {
	storage.TeamRepository = &TeamRepository{teams: make(map[string]storage.Team)}
}

func (t *TeamRepository) Insert(team storage.Team) error {
	_, ok := t.teams[team.Name]
	if ok {
		return storage.ErrTeamAlreadyExists
	}
	t.teams[team.Name] = team
	return nil
}

func (t *TeamRepository) FindAll() ([]storage.Team, error) {
	list := make([]storage.Team, len(t.teams))
	i := 0
	for _, team := range t.teams {
		list[i] = team
		i++
	}
	return list, nil
}

func (t *TeamRepository) FindByName(name string) (*storage.Team, error) {
	team, ok := t.teams[name]
	if !ok {
		return nil, storage.ErrTeamNotFound
	}
	return &team, nil
}

func (t *TeamRepository) Delete(team storage.Team) error {
	_, err := t.FindByName(team.Name)
	if err != nil {
		return err
	}
	delete(t.teams, team.Name)
	return nil
}

func (t *TeamRepository) Reset() {
	t.teams = make(map[string]storage.Team)
}
