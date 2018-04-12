// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"sync"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/quota"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

type authQuotaService struct {
	storage authTypes.AuthQuotaStorage
	mutex   *sync.Mutex
}

// ReserveApp reserves an app for the user, reserving it in the database. It's
// used to reserve the app in the user quota, returning an error when there
// isn't any space available.
func (s *authQuotaService) ReserveApp(user *authTypes.User, authQuota *authTypes.AuthQuota) error {
	_, err := checkUser(user.Email)
	if err != nil {
		return err
	}
	err = s.storage.IncInUse(user, authQuota, 1)
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
	user, err := GetUserByEmail(user.Email)
	if err != nil {
		return err
	}
	if user.Quota.InUse == 0 {
		return authTypes.ErrCantRelease
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Update(
		bson.M{"email": user.Email, "quota.inuse": user.Quota.InUse},
		bson.M{"$inc": bson.M{"quota.inuse": -1}},
	)
	for err == mgo.ErrNotFound {
		user, err = GetUserByEmail(user.Email)
		if err != nil {
			return err
		}
		if user.Quota.InUse == 0 {
			return authTypes.ErrCantRelease
		}
		err = conn.Users().Update(
			bson.M{"email": user.Email, "quota.inuse": user.Quota.InUse},
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
		return authTypes.ErrLimitLowerThanAllocated
	}
	user.Quota.Limit = limit
	return user.Update()
}
