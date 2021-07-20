// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/app/image"
)

const platformImageCollectionName = "platform_images"

var _ image.PlatformImageStorage = (*PlatformImageStorage)(nil)

type PlatformImageStorage struct{}

func platformImageCollection(conn *db.Storage) *dbStorage.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	c := conn.Collection(platformImageCollectionName)
	c.EnsureIndex(nameIndex)
	return c
}

func (s *PlatformImageStorage) Upsert(ctx context.Context, name string) (*image.PlatformImage, error) {
	query := bson.M{"name": name}

	span := newMongoDBSpan(ctx, mongoSpanUpsert, platformImageCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	defer conn.Close()
	dbChange := mgo.Change{
		Update:    bson.M{"$inc": bson.M{"count": 1}},
		ReturnNew: true,
		Upsert:    true,
	}
	var p image.PlatformImage
	_, err = platformImageCollection(conn).Find(query).Apply(dbChange, &p)
	span.SetError(err)
	return &p, err
}

func (s *PlatformImageStorage) FindByName(ctx context.Context, name string) (*image.PlatformImage, error) {
	query := bson.M{"name": name}

	span := newMongoDBSpan(ctx, mongoSpanFindOne, platformImageCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	var p image.PlatformImage
	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	defer conn.Close()
	err = platformImageCollection(conn).Find(query).One(&p)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = image.ErrPlatformImageNotFound
		}
		span.SetError(err)
		return nil, err
	}
	return &p, nil
}

func (s *PlatformImageStorage) Append(ctx context.Context, name string, image string) error {
	query := bson.M{"name": name}

	span := newMongoDBSpan(ctx, mongoSpanUpsert, platformImageCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	bulk := platformImageCollection(conn).Bulk()
	bulk.Upsert(query, bson.M{"$pull": bson.M{"images": image}})
	bulk.Upsert(query, bson.M{"$push": bson.M{"images": image}})
	_, err = bulk.Run()

	span.SetError(err)

	return err
}

func (s *PlatformImageStorage) Delete(ctx context.Context, name string) error {
	query := bson.M{"name": name}

	span := newMongoDBSpan(ctx, mongoSpanDelete, platformImageCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	err = platformImageCollection(conn).Remove(bson.M{"name": name})
	if err == mgo.ErrNotFound {
		return image.ErrPlatformImageNotFound
	}
	span.SetError(err)
	return err
}
