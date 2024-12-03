// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/storage/storagetest"
	appTypes "github.com/tsuru/tsuru/types/app"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

type appStorage struct{}

func (s *appStorage) Create(ctx context.Context, app *appTypes.App) error {
	appCollection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	_, err = appCollection.InsertOne(ctx, app)
	return err
}

func (s *appStorage) Remove(ctx context.Context, app *appTypes.App) error {
	appCollection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}

	_, err = appCollection.DeleteOne(ctx, mongoBSON.M{"name": app.Name})
	return err
}

var _ = check.Suite(&storagetest.AppQuotaSuite{
	AppStorage:      &appStorage{},
	AppQuotaStorage: appQuotaStorage(),
	SuiteHooks:      &mongodbBaseTest{},
})
