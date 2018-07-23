// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/app/image"
)

var _ image.PlatformImageStorage = &PlatformImageStorage{}

type PlatformImageStorage struct{}

type platformImage struct {
	Name   string `bson:"_id"`
	Images []string
	Count  int
}

func platformImageCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection("platform_images")
}

func (s *PlatformImageStorage) Upsert(name string) (*image.PlatformImage, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	dbChange := mgo.Change{
		Update:    bson.M{"$inc": bson.M{"count": 1}},
		ReturnNew: true,
		Upsert:    true,
	}
	var p platformImage
	_, err = platformImageCollection(conn).FindId(name).Apply(dbChange, &p)
	platform := image.PlatformImage(p)
	return &platform, err
}

func (s *PlatformImageStorage) FindByName(name string) (*image.PlatformImage, error) {
	var p platformImage
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = platformImageCollection(conn).FindId(name).One(&p)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = image.ErrPlatformImageNotFound
		}
		return nil, err
	}
	platform := image.PlatformImage(p)
	return &platform, nil
}

func (s *PlatformImageStorage) Append(name string, image string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	bulk := platformImageCollection(conn).Bulk()
	bulk.Upsert(bson.M{"_id": name}, bson.M{"$pull": bson.M{"images": image}})
	bulk.Upsert(bson.M{"_id": name}, bson.M{"$push": bson.M{"images": image}})
	_, err = bulk.Run()
	return err
}

func (s *PlatformImageStorage) Delete(name string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = platformImageCollection(conn).RemoveId(name)
	if err == mgo.ErrNotFound {
		return image.ErrPlatformImageNotFound
	}
	return err
}
