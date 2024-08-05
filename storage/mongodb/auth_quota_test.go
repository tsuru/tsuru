// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/storage/storagetest"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

type userStorage struct{}

func (s *userStorage) Create(user *auth.User) error {
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return err
	}
	_, err = usersCollection.InsertOne(context.TODO(), user)
	return err
}

func (s *userStorage) Remove(user *auth.User) error {
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return err
	}

	_, err = usersCollection.DeleteOne(context.TODO(), mongoBSON.M{"email": user.Email})
	return err
}

var _ = check.Suite(&storagetest.UserQuotaSuite{
	UserStorage:      &userStorage{},
	UserQuotaStorage: authQuotaStorage(),
	SuiteHooks:       &mongodbBaseTest{},
})
