// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/types/auth"
)

var _ auth.UserStorage = &UserStorage{}

type UserStorage struct{}

func (s *UserStorage) Create(user interface{}) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Insert(user)
	return err
}

func (s *UserStorage) Remove(user interface{}) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Remove(user)
	return err
}
