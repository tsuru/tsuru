// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/storage/storagetest"
	"gopkg.in/check.v1"
)

type userStorage struct{}

func (s *userStorage) Create(user *auth.User) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Insert(user)
	return err
}

func (s *userStorage) Remove(user *auth.User) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Remove(user)
	return err
}

var _ = check.Suite(&storagetest.UserQuotaSuite{
	UserStorage:      &userStorage{},
	UserQuotaStorage: authQuotaStorage(),
	SuiteHooks:       &mongodbBaseTest{},
})
