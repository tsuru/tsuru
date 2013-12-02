// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/quota"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

// ReserveApp reserves an app for the user, reserving it in the database. It's
// used to reserve the app in the user quota, returning an error when there
// isn't any space available.
func ReserveApp(user *User) error {
	user, err := checkUser(user.Email)
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Update(
		bson.M{"email": user.Email, "quota.inuse": user.InUse},
		bson.M{"$inc": bson.M{"quota.inuse": 1}},
	)
	for err == mgo.ErrNotFound {
		user, err = checkUser(user.Email)
		if err != nil {
			return err
		}
		err = conn.Users().Update(
			bson.M{"email": user.Email, "quota.inuse": user.InUse},
			bson.M{"$inc": bson.M{"quota.inuse": 1}},
		)
	}
	return err
}

func checkUser(email string) (*User, error) {
	user, err := GetUserByEmail(email)
	if err != nil {
		return nil, err
	}
	if user.Quota.Limit == user.Quota.InUse {
		return nil, &quota.QuotaExceededError{
			Available: 0, Requested: 1,
		}
	}
	return user, nil
}
