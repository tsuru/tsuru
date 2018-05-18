// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/storage/storagetest"
	"gopkg.in/check.v1"
)

type appStorage struct{}

func (s *appStorage) Create(app *app.App) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Insert(app)
	return err
}

func (s *appStorage) Remove(app *app.App) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Remove(app)
	return err
}

var _ = check.Suite(&storagetest.AppQuotaSuite{
	AppStorage:      &appStorage{},
	AppQuotaStorage: &appQuotaStorage{},
	SuiteHooks:      &mongodbBaseTest{},
})
