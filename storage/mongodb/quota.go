// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/types/quota"
)

var _ quota.QuotaStorage = &quotaStorage{}

type quotaStorage struct {
	collection string
	query      func(string) bson.M
}

type quotaObject struct {
	Quota quota.Quota
}

func (s *quotaStorage) SetLimit(name string, limit int) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.Get(name)
	if err != nil {
		return err
	}
	err = conn.Collection(s.collection).Update(
		s.query(name),
		bson.M{"$set": bson.M{"quota.limit": limit}},
	)
	return err
}

func (s *quotaStorage) Set(name string, inUse int) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.Get(name)
	if err != nil {
		return err
	}
	err = conn.Collection(s.collection).Update(
		s.query(name),
		bson.M{"$set": bson.M{"quota.inuse": inUse}},
	)
	return err
}

func (s *quotaStorage) Get(name string) (*quota.Quota, error) {
	var obj quotaObject
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Collection(s.collection).Find(s.query(name)).One(&obj)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, quota.ErrQuotaNotFound
		}
		return nil, err
	}
	return &obj.Quota, nil
}
