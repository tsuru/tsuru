// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import "github.com/tsuru/tsuru/types"

type TeamStorage interface {
	Insert(types.Team) error
	FindAll() ([]types.Team, error)
	FindByName(string) (*types.Team, error)
	FindByNames([]string) ([]types.Team, error)
	Delete(types.Team) error
}
