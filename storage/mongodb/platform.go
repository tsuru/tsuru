// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/app"
)

var _ app.PlatformStorage = (*PlatformStorage)(nil)

const platformsCollectionName = "platforms"

type PlatformStorage struct{}

type platform struct {
	Name     string `bson:"_id"`
	Disabled bool   `bson:",omitempty"`
}

func platformsCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection(platformsCollectionName)
}

func (s *PlatformStorage) Insert(ctx context.Context, p app.Platform) error {
	span := newMongoDBSpan(ctx, mongoSpanInsert, platformsCollectionName)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	err = platformsCollection(conn).Insert(platform(p))
	span.SetError(err)
	if mgo.IsDup(err) {
		return app.ErrDuplicatePlatform
	}
	return err
}

func (s *PlatformStorage) FindByName(ctx context.Context, name string) (*app.Platform, error) {
	span := newMongoDBSpan(ctx, mongoSpanFindID, platformsCollectionName)
	defer span.Finish()

	var p platform
	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	defer conn.Close()
	err = platformsCollection(conn).FindId(name).One(&p)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = app.ErrPlatformNotFound
		}
		span.SetError(err)
		return nil, err
	}
	platform := app.Platform(p)
	return &platform, nil
}

func (s *PlatformStorage) FindAll(ctx context.Context) ([]app.Platform, error) {
	return s.findByQuery(ctx, nil)
}

func (s *PlatformStorage) FindEnabled(ctx context.Context) ([]app.Platform, error) {
	query := bson.M{"$or": []bson.M{{"disabled": false}, {"disabled": bson.M{"$exists": false}}}}
	return s.findByQuery(ctx, query)
}

func (s *PlatformStorage) findByQuery(ctx context.Context, query bson.M) ([]app.Platform, error) {
	span := newMongoDBSpan(ctx, mongoSpanFindID, platformsCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	defer conn.Close()
	var platforms []platform
	err = platformsCollection(conn).Find(query).All(&platforms)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	appPlatforms := make([]app.Platform, len(platforms))
	for i, p := range platforms {
		appPlatforms[i] = app.Platform(p)
	}
	return appPlatforms, nil
}

func (s *PlatformStorage) Update(ctx context.Context, p app.Platform) error {
	span := newMongoDBSpan(ctx, mongoSpanUpdate, platformsCollectionName)
	span.SetMongoID(p.Name)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	err = platformsCollection(conn).Update(
		bson.M{"_id": p.Name},
		bson.M{"$set": bson.M{"disabled": p.Disabled}},
	)
	span.SetError(err)
	return err
}

func (s *PlatformStorage) Delete(ctx context.Context, p app.Platform) error {
	span := newMongoDBSpan(ctx, mongoSpanDeleteID, platformsCollectionName)
	span.SetMongoID(p.Name)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	err = platformsCollection(conn).RemoveId(p.Name)
	if err == mgo.ErrNotFound {
		span.SetError(err)
		return app.ErrPlatformNotFound
	}
	return err
}
