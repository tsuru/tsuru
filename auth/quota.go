// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/quota"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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

// ReleaseApp releases an app from the user list, releasing the quota spot for
// another app.
func ReleaseApp(user *User) error {
	errCantRelease := errors.New("Cannot release unreserved app")
	user, err := GetUserByEmail(user.Email)
	if err != nil {
		return err
	}
	if user.Quota.InUse == 0 {
		return errCantRelease
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Update(
		bson.M{"email": user.Email, "quota.inuse": user.InUse},
		bson.M{"$inc": bson.M{"quota.inuse": -1}},
	)
	for err == mgo.ErrNotFound {
		user, err = GetUserByEmail(user.Email)
		if err != nil {
			return err
		}
		if user.Quota.InUse == 0 {
			return errCantRelease
		}
		err = conn.Users().Update(
			bson.M{"email": user.Email, "quota.inuse": user.InUse},
			bson.M{"$inc": bson.M{"quota.inuse": -1}},
		)
	}
	return err
}

// ChangeQuota redefines the limit of the user. The new limit must be bigger
// than or equal to the current number of apps of the user. The new limit maybe
// smaller than 0, which mean that the user should have an unlimited number of
// apps.
func ChangeQuota(user *User, limit int) error {
	if limit < 0 {
		limit = -1
	} else if limit < user.Quota.InUse {
		return errors.New("new limit is lesser than the current allocated value")
	}
	user.Quota.Limit = limit
	return user.Update()
}
