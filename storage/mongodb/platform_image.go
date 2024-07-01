// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	appImage "github.com/tsuru/tsuru/app/image"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/app/image"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	platformImageCollectionNameV1 = "platform_images"
	platformImageCollectionNameV2 = "platform_images_v2"
)

var _ image.PlatformImageStorage = &PlatformImageStorage{}

type PlatformImageStorage struct {
	migrated sync.Once
}

type platformImageV1 struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"`
	Name         string
	LegacyImages []string `bson:"images,omitempty"`
	Versions     []image.RegistryVersion
	Count        int
}

type platformImageV2 struct {
	Name     string `bson:"_id"`
	Versions []image.RegistryVersion
	Count    int
}

func (s *PlatformImageStorage) migrate(ctx context.Context) error {
	var err error
	s.migrated.Do(func() {
		err = s.migrateV1ToV2(ctx)
	})

	return err
}

func (s *PlatformImageStorage) migrateV1ToV2(ctx context.Context) error {
	collectionV1, err := storagev2.Collection(platformImageCollectionNameV1)
	if err != nil {
		return err
	}

	collectionV2, err := storagev2.Collection(platformImageCollectionNameV2)
	if err != nil {
		return err
	}

	countV1, err := collectionV1.CountDocuments(ctx, mongoBSON.M{})
	if err != nil {
		return err
	}

	countV2, err := collectionV2.CountDocuments(ctx, mongoBSON.M{})
	if err != nil {
		return err
	}
	if countV1 == countV2 {
		return nil
	}

	var pV1 []platformImageV1
	cursor, err := collectionV1.Find(ctx, mongoBSON.M{})
	if err != nil {
		return err
	}
	err = cursor.All(ctx, &pV1)
	if err != nil {
		return err
	}

	for _, p := range pV1 {
		pV2 := platformImageV2{
			Name:     p.Name,
			Versions: p.Versions,
			Count:    p.Count,
		}

		for _, legacyImage := range p.LegacyImages {
			_, _, tag := appImage.ParseImageParts(legacyImage)
			version, _ := strconv.Atoi(strings.TrimPrefix(tag, "v"))
			pV2.Versions = append(pV2.Versions, image.RegistryVersion{
				Version: version,
				Images:  []string{legacyImage},
			})
		}

		sort.Slice(pV2.Versions, func(i, j int) bool {
			return pV2.Versions[i].Version < pV2.Versions[j].Version
		})

		opts := options.FindOneAndReplace().SetUpsert(true)
		err = collectionV2.FindOneAndReplace(ctx, mongoBSON.M{"_id": p.Name}, pV2, opts).Err()
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *PlatformImageStorage) Upsert(ctx context.Context, name string) (*image.PlatformImage, error) {
	err := s.migrate(ctx)
	if err != nil {
		return nil, err
	}
	query := mongoBSON.M{"_id": name}

	span := newMongoDBSpan(ctx, mongoSpanUpsert, platformImageCollectionNameV2)
	span.SetQueryStatement(query)
	defer span.Finish()

	collection, err := storagev2.Collection(platformImageCollectionNameV2)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	update := mongoBSON.M{
		"$inc": mongoBSON.M{"count": 1},
	}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)

	var p platformImageV2

	err = collection.FindOneAndUpdate(ctx, mongoBSON.M{"_id": name}, update, opts).Decode(&p)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	pi := image.PlatformImage(p)
	return &pi, err
}

func (s *PlatformImageStorage) FindByName(ctx context.Context, name string) (*image.PlatformImage, error) {
	err := s.migrate(ctx)
	if err != nil {
		return nil, err
	}

	query := mongoBSON.M{"_id": name}

	span := newMongoDBSpan(ctx, mongoSpanFindOne, platformImageCollectionNameV2)
	span.SetQueryStatement(query)
	defer span.Finish()

	var p platformImageV2
	collection, err := storagev2.Collection(platformImageCollectionNameV2)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	err = collection.FindOne(ctx, query).Decode(&p)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			err = image.ErrPlatformImageNotFound
		}
		span.SetError(err)
		return nil, err
	}
	pi := image.PlatformImage(p)
	return &pi, nil
}

func (s *PlatformImageStorage) Append(ctx context.Context, name string, version int, images []string) error {
	err := s.migrate(ctx)
	if err != nil {
		return err
	}

	query := mongoBSON.M{"_id": name, "versions.version": version}
	span := newMongoDBSpan(ctx, mongoSpanUpsert, platformImageCollectionNameV2)
	span.SetQueryStatement(query)
	defer span.Finish()

	collection, err := storagev2.Collection(platformImageCollectionNameV2)
	if err != nil {
		span.SetError(err)
		return err
	}

	result, err := collection.UpdateOne(ctx, query, mongoBSON.M{"$push": mongoBSON.M{"versions.$.images": mongoBSON.M{"$each": images}}})
	if err != nil {
		span.SetError(err)
		return err
	}

	if result.MatchedCount == 0 {
		_, err = collection.UpdateOne(ctx, mongoBSON.M{"_id": name},
			mongoBSON.M{"$push": mongoBSON.M{
				"versions": image.RegistryVersion{Version: version, Images: images},
			}},
		)
	}

	return err
}

func (s *PlatformImageStorage) Delete(ctx context.Context, name string) error {
	err := s.migrate(ctx)
	if err != nil {
		return err
	}

	queryV1 := mongoBSON.M{"name": name}
	queryV2 := mongoBSON.M{"_id": name}

	span1 := newMongoDBSpan(ctx, mongoSpanDelete, platformImageCollectionNameV1)
	span1.SetQueryStatement(queryV1)
	defer span1.Finish()

	span2 := newMongoDBSpan(ctx, mongoSpanDelete, platformImageCollectionNameV2)
	span2.SetQueryStatement(queryV2)
	defer span2.Finish()

	collectionV1, err := storagev2.Collection(platformImageCollectionNameV1)
	if err != nil {
		span1.SetError(err)
		return err
	}

	collectionV2, err := storagev2.Collection(platformImageCollectionNameV2)
	if err != nil {
		span2.SetError(err)
		return err
	}

	_, err = collectionV1.DeleteOne(ctx, queryV1)
	if err != nil && err != mongo.ErrNoDocuments {
		span1.SetError(err)
		return err
	}

	result, err := collectionV2.DeleteOne(ctx, queryV2)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return image.ErrPlatformImageNotFound
		}
		span2.SetError(err)
		return err
	}

	if result.DeletedCount == 0 {
		return image.ErrPlatformImageNotFound
	}

	return nil
}
