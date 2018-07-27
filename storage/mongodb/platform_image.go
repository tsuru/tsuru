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

func platformImageCollection(conn *db.Storage) *dbStorage.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	c := conn.Collection("platform_images")
	c.EnsureIndex(nameIndex)
	return c
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
	var p image.PlatformImage
	_, err = platformImageCollection(conn).Find(bson.M{"name": name}).Apply(dbChange, &p)
	return &p, err
}

func (s *PlatformImageStorage) FindByName(name string) (*image.PlatformImage, error) {
	var p image.PlatformImage
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = platformImageCollection(conn).Find(bson.M{"name": name}).One(&p)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = image.ErrPlatformImageNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *PlatformImageStorage) Append(name string, image string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	bulk := platformImageCollection(conn).Bulk()
	bulk.Upsert(bson.M{"name": name}, bson.M{"$pull": bson.M{"images": image}})
	bulk.Upsert(bson.M{"name": name}, bson.M{"$push": bson.M{"images": image}})
	_, err = bulk.Run()
	return err
}

func (s *PlatformImageStorage) Delete(name string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = platformImageCollection(conn).Remove(bson.M{"name": name})
	if err == mgo.ErrNotFound {
		return image.ErrPlatformImageNotFound
	}
	return err
}
