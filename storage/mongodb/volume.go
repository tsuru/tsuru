// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/volume"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	volumeCollectionName      = "volumes"
	volumeBindsCollectionName = "volume_binds"
)

var _ volume.VolumeStorage = &volumeStorage{}

func volumesCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection(volumeCollectionName)
}
func volumeBindsCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection(volumeBindsCollectionName)
}

type volumeStorage struct {
}

func (*volumeStorage) Save(ctx context.Context, v *volume.Volume) error {
	span := newMongoDBSpan(ctx, mongoSpanUpsertID, volumeCollectionName)
	defer span.Finish()
	span.SetMongoID(v.Name)

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()

	_, err = volumesCollection(conn).UpsertId(v.Name, v)
	span.SetError(err)
	return errors.WithStack(err)
}

func (*volumeStorage) Delete(ctx context.Context, v *volume.Volume) error {
	span := newMongoDBSpan(ctx, mongoSpanDeleteID, volumeCollectionName)
	defer span.Finish()
	span.SetMongoID(v.Name)

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()

	err = volumesCollection(conn).RemoveId(v.Name)
	span.SetError(err)
	return errors.WithStack(err)
}

func (*volumeStorage) Get(ctx context.Context, name string) (*volume.Volume, error) {
	span := newMongoDBSpan(ctx, mongoSpanFindID, volumeCollectionName)
	defer span.Finish()
	span.SetMongoID(name)

	collection, err := storagev2.Collection(volumeCollectionName)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	var v volume.Volume
	err = collection.FindOne(ctx, mongoBSON.M{"_id": name}).Decode(&v)
	if err == mongo.ErrNoDocuments {
		return nil, volume.ErrVolumeNotFound
	}
	if err != nil {
		span.SetError(err)
		return nil, errors.WithStack(err)
	}

	return &v, nil
}

func (*volumeStorage) ListByFilter(ctx context.Context, f *volume.Filter) ([]volume.Volume, error) {
	span := newMongoDBSpan(ctx, mongoSpanFind, volumeCollectionName)
	defer span.Finish()

	collection, err := storagev2.Collection(volumeCollectionName)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	query := mongoBSON.M{}
	if f != nil {
		orQueries := []mongoBSON.M{}

		if len(f.Names) > 0 {
			orQueries = append(orQueries, mongoBSON.M{"_id": mongoBSON.M{"$in": f.Names}})
		}

		if len(f.Pools) > 0 {
			orQueries = append(orQueries, mongoBSON.M{"pool": mongoBSON.M{"$in": f.Pools}})
		}

		if len(f.Teams) > 0 {
			orQueries = append(orQueries, mongoBSON.M{"teamowner": mongoBSON.M{"$in": f.Teams}})
		}

		query["$or"] = orQueries
	}
	span.SetQueryStatement(query)

	var volumes []volume.Volume

	cursor, err := collection.Find(ctx, query)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	err = cursor.All(ctx, &volumes)
	if err != nil {
		span.SetError(err)
		return nil, errors.WithStack(err)
	}
	return volumes, nil
}

func (*volumeStorage) InsertBind(ctx context.Context, b *volume.VolumeBind) error {
	span := newMongoDBSpan(ctx, mongoSpanInsert, volumeBindsCollectionName)
	defer span.Finish()
	span.SetMongoID(b.ID)

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()

	err = volumeBindsCollection(conn).Insert(b)
	if err != nil && mgo.IsDup(err) {
		return volume.ErrVolumeAlreadyBound
	}

	span.SetError(err)
	return errors.WithStack(err)
}

func (*volumeStorage) RemoveBind(ctx context.Context, id volume.VolumeBindID) error {
	span := newMongoDBSpan(ctx, mongoSpanDeleteID, volumeBindsCollectionName)
	defer span.Finish()
	span.SetMongoID(id)

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	err = volumeBindsCollection(conn).RemoveId(id)
	if err == mgo.ErrNotFound {
		return volume.ErrVolumeBindNotFound
	}
	span.SetError(err)
	return errors.WithStack(err)
}

func (*volumeStorage) Binds(ctx context.Context, volumeName string) ([]volume.VolumeBind, error) {
	span := newMongoDBSpan(ctx, mongoSpanFind, volumeBindsCollectionName)
	defer span.Finish()

	collection, err := storagev2.Collection(volumeBindsCollectionName)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	var binds []volume.VolumeBind
	query := mongoBSON.M{"_id.volume": volumeName}
	span.SetQueryStatement(query)

	cursor, err := collection.Find(ctx, query)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	err = cursor.All(ctx, &binds)
	if err != nil {
		span.SetError(err)
		return nil, errors.WithStack(err)
	}

	return binds, nil
}

func (*volumeStorage) BindsForApp(ctx context.Context, volumeName, appName string) ([]volume.VolumeBind, error) {
	span := newMongoDBSpan(ctx, mongoSpanFind, volumeBindsCollectionName)
	defer span.Finish()

	collection, err := storagev2.Collection(volumeBindsCollectionName)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	var binds []volume.VolumeBind
	query := mongoBSON.M{"_id.app": appName}
	if volumeName != "" {
		query["_id.volume"] = volumeName
	}
	span.SetQueryStatement(query)

	cursor, err := collection.Find(ctx, query)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	err = cursor.All(ctx, &binds)
	if err != nil {
		span.SetError(err)
		return nil, errors.WithStack(err)
	}

	return binds, nil
}
func (*volumeStorage) RenameTeam(ctx context.Context, oldName, newName string) error {
	span := newMongoDBSpan(ctx, mongoSpanUpdateAll, volumeCollectionName)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()

	query := bson.M{"teamowner": oldName}
	span.SetQueryStatement(query)
	_, err = volumesCollection(conn).UpdateAll(query, bson.M{"$set": bson.M{"teamowner": newName}})
	span.SetError(err)
	return errors.WithStack(err)
}
