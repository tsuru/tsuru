// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"strconv"
	"strings"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	appImage "github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/app/image"
)

const platformImageCollectionName = "platform_images"

var _ image.PlatformImageStorage = &PlatformImageStorage{}

type PlatformImageStorage struct{}

type platformImage struct {
	Name     string
	Versions []image.RegistryVersion
	Count    int
}

func (pi *platformImage) SetBSON(raw bson.Raw) error {
	parsedRaw := map[string]bson.Raw{}
	err := raw.Unmarshal(&parsedRaw)
	if err != nil {
		return err
	}
	if value, ok := parsedRaw["name"]; ok {
		value.Unmarshal(&pi.Name)
	}
	if value, ok := parsedRaw["count"]; ok {
		value.Unmarshal(&pi.Count)
	}
	if value, ok := parsedRaw["versions"]; ok {
		value.Unmarshal(&pi.Versions)
	}
	if value, ok := parsedRaw["images"]; ok {
		var legacyImages []string
		err = value.Unmarshal(&legacyImages)
		if err != nil {
			return err
		}

		var legacyVersions []image.RegistryVersion
		for _, legacyImage := range legacyImages {
			_, _, tag := appImage.ParseImageParts(legacyImage)
			version, _ := strconv.Atoi(strings.TrimPrefix(tag, "v"))
			legacyVersions = append(legacyVersions, image.RegistryVersion{
				Version: version,
				Images:  []string{legacyImage},
			})
		}
		pi.Versions = append(legacyVersions, pi.Versions...)
	}
	return nil
}

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
		Update: bson.M{
			"$inc": bson.M{"count": 1},
		},
		ReturnNew: true,
		Upsert:    true,
	}
	var p platformImage
	_, err = platformImageCollection(conn).Find(query).Apply(dbChange, &p)
	span.SetError(err)
	pi := image.PlatformImage(p)
	return &pi, err
}

func (s *PlatformImageStorage) FindByName(ctx context.Context, name string) (*image.PlatformImage, error) {
	query := bson.M{"name": name}

	span := newMongoDBSpan(ctx, mongoSpanFindOne, platformImageCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	var p platformImage
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
	pi := image.PlatformImage(p)
	return &pi, nil
}

func (s *PlatformImageStorage) Append(ctx context.Context, name string, version int, images []string) error {
	query := bson.M{"name": name, "versions.version": version}
	span := newMongoDBSpan(ctx, mongoSpanUpsert, platformImageCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()

	coll := platformImageCollection(conn)
	ci, err := coll.UpdateAll(query, bson.M{"$push": bson.M{"versions.$.images": bson.M{"$each": images}}})
	if err != nil {
		return err
	}

	if ci.Matched == 0 {
		err = coll.Update(bson.M{"name": name},
			bson.M{"$push": bson.M{
				"versions": image.RegistryVersion{Version: version, Images: images},
			}},
		)
	}
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
