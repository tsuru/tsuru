// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

var _ authTypes.AuthQuotaStorage = &AuthQuotaStorage{}

type AuthQuotaStorage struct{}

func (s *AuthQuotaStorage) IncInUse(email string, quota *authTypes.AuthQuota, quantity int) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Update(
		bson.M{"email": email, "quota.inuse": quota.InUse},
		bson.M{"$inc": bson.M{"quota.inuse": quantity}},
	)
	return err
}

func (s *AuthQuotaStorage) SetLimit(email string, quota *authTypes.AuthQuota, quantity int) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Update(
		bson.M{"email": email},
		bson.M{"$set": bson.M{"quota.limit": quantity}},
	)
	return err
}
