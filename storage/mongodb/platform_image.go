// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"sort"
	"strconv"
	"strings"

	appImage "github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/app/image"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var _ image.PlatformImageStorage = &PlatformImageStorage{}

type PlatformImageStorage struct{}

type platformImage struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"`
	Name         string
	LegacyImages []string `bson:"images,omitempty"`
	Versions     []image.RegistryVersion
	Count        int
}

func (s *PlatformImageStorage) Upsert(ctx context.Context, name string) (*image.PlatformImage, error) {
	query := mongoBSON.M{"name": name}

	collection, err := storagev2.PlatformImagesCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanUpsert, collection.Name())
	span.SetQueryStatement(query)
	defer span.Finish()

	update := mongoBSON.M{
		"$inc": mongoBSON.M{"count": 1},
	}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)

	var p platformImage

	err = collection.FindOneAndUpdate(ctx, mongoBSON.M{"name": name}, update, opts).Decode(&p)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	return convertPlatformImage(p), nil
}

func (s *PlatformImageStorage) FindByName(ctx context.Context, name string) (*image.PlatformImage, error) {
	query := mongoBSON.M{"name": name}

	var p platformImage
	collection, err := storagev2.PlatformImagesCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFindOne, collection.Name())
	span.SetQueryStatement(query)
	defer span.Finish()

	err = collection.FindOne(ctx, query).Decode(&p)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			err = image.ErrPlatformImageNotFound
		}
		span.SetError(err)
		return nil, err
	}

	return convertPlatformImage(p), nil
}

func (s *PlatformImageStorage) Append(ctx context.Context, name string, version int, images []string) error {
	query := mongoBSON.M{"name": name, "versions.version": version}

	collection, err := storagev2.PlatformImagesCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanUpsert, collection.Name())
	span.SetQueryStatement(query)
	defer span.Finish()

	result, err := collection.UpdateOne(ctx, query, mongoBSON.M{"$push": mongoBSON.M{"versions.$.images": mongoBSON.M{"$each": images}}})
	if err != nil {
		span.SetError(err)
		return err
	}

	if result.MatchedCount == 0 {
		_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": name},
			mongoBSON.M{"$push": mongoBSON.M{
				"versions": image.RegistryVersion{Version: version, Images: images},
			}},
		)

		if err != nil {
			span.SetError(err)
			return err
		}
	}

	return nil
}

func (s *PlatformImageStorage) Delete(ctx context.Context, name string) error {
	query := mongoBSON.M{"name": name}

	collection, err := storagev2.PlatformImagesCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanDelete, collection.Name())
	span.SetQueryStatement(query)
	defer span.Finish()

	result, err := collection.DeleteOne(ctx, query)
	if err != nil && err != mongo.ErrNoDocuments {
		span.SetError(err)
		return err
	}

	if result.DeletedCount == 0 {
		return image.ErrPlatformImageNotFound
	}

	return nil
}

func convertPlatformImage(p platformImage) *image.PlatformImage {
	pi := image.PlatformImage{
		Name:     p.Name,
		Versions: p.Versions,
		Count:    p.Count,
	}

	for _, legacyImage := range p.LegacyImages {
		_, _, tag := appImage.ParseImageParts(legacyImage)
		version, _ := strconv.Atoi(strings.TrimPrefix(tag, "v"))

		pi.Versions = append(pi.Versions, image.RegistryVersion{
			Version: version,
			Images:  []string{legacyImage},
		})
	}

	sort.Slice(pi.Versions, func(i, j int) bool {
		return pi.Versions[i].Version < pi.Versions[j].Version
	})

	return &pi
}
