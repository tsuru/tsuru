// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	mgo "github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/app"
)

var _ app.PlatformStorage = &PlatformStorage{}

type PlatformStorage struct{}

type platform struct {
	Name     string `bson:"_id"`
	Disabled bool   `bson:",omitempty"`
}

func platformsCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection("platforms")
}

func (s *PlatformStorage) Insert(p app.Platform) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = platformsCollection(conn).Insert(platform(p))
	if mgo.IsDup(err) {
		return app.ErrDuplicatePlatform
	}
	return err
}

func (s *PlatformStorage) FindByName(name string) (*app.Platform, error) {
	var p platform
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = platformsCollection(conn).FindId(name).One(&p)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = app.ErrPlatformNotFound
		}
		return nil, err
	}
	platform := app.Platform(p)
	return &platform, nil
}

func (s *PlatformStorage) FindAll() ([]app.Platform, error) {
	return s.findByQuery(nil)
}

func (s *PlatformStorage) FindEnabled() ([]app.Platform, error) {
	query := bson.M{"$or": []bson.M{{"disabled": false}, {"disabled": bson.M{"$exists": false}}}}
	return s.findByQuery(query)
}

func (s *PlatformStorage) findByQuery(query bson.M) ([]app.Platform, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var platforms []platform
	err = platformsCollection(conn).Find(query).All(&platforms)
	if err != nil {
		return nil, err
	}
	appPlatforms := make([]app.Platform, len(platforms))
	for i, p := range platforms {
		appPlatforms[i] = app.Platform(p)
	}
	return appPlatforms, nil
}

func (s *PlatformStorage) Update(p app.Platform) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return platformsCollection(conn).Update(bson.M{"_id": p.Name}, bson.M{"$set": bson.M{"disabled": p.Disabled}})
}

func (s *PlatformStorage) Delete(p app.Platform) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = platformsCollection(conn).RemoveId(p.Name)
	if err == mgo.ErrNotFound {
		return app.ErrPlatformNotFound
	}
	return err
}
