// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	mgo "github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/types/app"
)

var _ app.QuotaStorage = &AppQuotaStorage{}

type AppQuotaStorage struct{}

// Fake implementation for storage quota.
type _app struct {
	name  string
	Quota app.Quota
}

func (s *AppQuotaStorage) IncInUse(appName string, quantity int) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.FindByAppName(appName)
	if err != nil {
		return err
	}
	err = conn.Apps().Update(
		bson.M{"name": appName},
		bson.M{"$inc": bson.M{"quota.inuse": quantity}},
	)
	return err
}

func (s *AppQuotaStorage) SetLimit(appName string, limit int) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.FindByAppName(appName)
	if err != nil {
		return err
	}
	err = conn.Apps().Update(
		bson.M{"name": appName},
		bson.M{"$set": bson.M{"quota.limit": limit}},
	)
	return err
}

func (s *AppQuotaStorage) SetInUse(appName string, inUse int) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.FindByAppName(appName)
	if err != nil {
		return err
	}
	err = conn.Apps().Update(
		bson.M{"name": appName},
		bson.M{"$set": bson.M{"quota.inuse": inUse}},
	)
	return err
}

func (s *AppQuotaStorage) FindByAppName(appName string) (*app.Quota, error) {
	var a _app
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Apps().Find(bson.M{"name": appName}).One(&a)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, app.ErrAppNotFound
		}
		return nil, err
	}
	return &a.Quota, nil
}
