// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/types/app"
)

var _ app.AppStorage = &AppStorage{}

type AppStorage struct{}

func (s *AppStorage) Create(app interface{}) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Insert(app)
	return err
}

func (s *AppStorage) Remove(app interface{}) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Remove(app)
	return err
}
