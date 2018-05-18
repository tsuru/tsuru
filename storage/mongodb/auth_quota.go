// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/quota"
)

var _ quota.UserQuotaStorage = &userQuotaStorage{}

type userQuotaStorage struct{}

type _user struct {
	email string      `bson:"_id"`
	Quota quota.Quota `bson:"quota"`
}

func (s *userQuotaStorage) IncInUse(email string, quantity int) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.FindByUserEmail(email)
	if err != nil {
		return err
	}
	err = conn.Users().Update(
		bson.M{"email": email},
		bson.M{"$inc": bson.M{"quota.inuse": quantity}},
	)
	return err
}

func (s *userQuotaStorage) SetLimit(email string, quantity int) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.FindByUserEmail(email)
	if err != nil {
		return err
	}
	err = conn.Users().Update(
		bson.M{"email": email},
		bson.M{"$set": bson.M{"quota.limit": quantity}},
	)
	return err
}

func (s *userQuotaStorage) FindByUserEmail(email string) (*quota.Quota, error) {
	var user _user
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": email}).One(&user)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, authTypes.ErrUserNotFound
		}
		return nil, err
	}
	return &user.Quota, nil
}
