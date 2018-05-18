// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/quota"
)

var _ quota.AppQuotaStorage = &appQuotaStorage{}

type appQuotaStorage struct{}

// Fake implementation for storage quota.
type _app struct {
	name  string
	Quota quota.Quota
}

func (s *appQuotaStorage) IncInUse(appName string, quantity int) error {
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

func (s *appQuotaStorage) SetLimit(appName string, limit int) error {
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

func (s *appQuotaStorage) SetInUse(appName string, inUse int) error {
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

func (s *appQuotaStorage) FindByAppName(appName string) (*quota.Quota, error) {
	var a _app
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Apps().Find(bson.M{"name": appName}).One(&a)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, appTypes.ErrAppNotFound
		}
		return nil, err
	}
	return &a.Quota, nil
}
