// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import "github.com/tsuru/tsuru/types/auth"

type TeamStorage interface {
	Insert(auth.Team) error
	FindAll() ([]auth.Team, error)
	FindByName(string) (*auth.Team, error)
	FindByNames([]string) ([]auth.Team, error)
	Delete(auth.Team) error
}
