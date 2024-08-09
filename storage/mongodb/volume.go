// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/volume"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var _ volume.VolumeStorage = &volumeStorage{}

type volumeStorage struct{}

func (*volumeStorage) Save(ctx context.Context, v *volume.Volume) error {
	collection, err := storagev2.VolumesCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanUpsertID, collection.Name())
	span.SetMongoID(v.Name)
	defer span.Finish()

	_, err = collection.ReplaceOne(ctx, mongoBSON.M{"_id": v.Name}, v, options.Replace().SetUpsert(true))

	if err != nil {
		span.SetError(err)
		return errors.WithStack(err)
	}

	return nil
}

func (*volumeStorage) Delete(ctx context.Context, v *volume.Volume) error {
	collection, err := storagev2.VolumesCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanDeleteID, collection.Name())
	defer span.Finish()
	span.SetMongoID(v.Name)

	_, err = collection.DeleteOne(ctx, mongoBSON.M{"_id": v.Name})

	if err != nil {
		span.SetError(err)
		return errors.WithStack(err)
	}

	return nil
}

func (*volumeStorage) Get(ctx context.Context, name string) (*volume.Volume, error) {

	collection, err := storagev2.VolumesCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFindID, collection.Name())
	defer span.Finish()
	span.SetMongoID(name)

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
	collection, err := storagev2.VolumesCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	defer span.Finish()

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
	collection, err := storagev2.VolumeBindsCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanInsert, collection.Name())
	defer span.Finish()
	span.SetMongoID(b.ID)

	_, err = collection.InsertOne(ctx, b)
	if err != nil && mongo.IsDuplicateKeyError(err) {
		return volume.ErrVolumeAlreadyBound
	}

	if err != nil {
		span.SetError(err)
		return errors.WithStack(err)
	}

	return nil
}

func (*volumeStorage) RemoveBind(ctx context.Context, id volume.VolumeBindID) error {
	collection, err := storagev2.VolumeBindsCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanDeleteID, collection.Name())
	defer span.Finish()
	span.SetMongoID(id)

	result, err := collection.DeleteOne(ctx, mongoBSON.M{"_id": id})
	if err == mongo.ErrNoDocuments {
		return volume.ErrVolumeBindNotFound
	}
	if err != nil {
		span.SetError(err)
		return errors.WithStack(err)
	}

	if result.DeletedCount == 0 {
		return volume.ErrVolumeBindNotFound
	}

	return nil
}

func (*volumeStorage) Binds(ctx context.Context, volumeName string) ([]volume.VolumeBind, error) {
	collection, err := storagev2.VolumeBindsCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	defer span.Finish()

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
	collection, err := storagev2.VolumeBindsCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	defer span.Finish()

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
	collection, err := storagev2.VolumesCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanUpdateAll, collection.Name())
	defer span.Finish()

	query := mongoBSON.M{"teamowner": oldName}
	span.SetQueryStatement(query)
	_, err = collection.UpdateMany(ctx, query, mongoBSON.M{"$set": mongoBSON.M{"teamowner": newName}})

	if err != nil {
		span.SetError(err)
		return errors.WithStack(err)
	}

	return nil
}
