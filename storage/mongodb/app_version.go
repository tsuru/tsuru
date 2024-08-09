// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"fmt"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	uuid "github.com/nu7hatch/gouuid"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/db/storagev2"
	appTypes "github.com/tsuru/tsuru/types/app"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	appVersionsCollectionName = "app_versions"
)

type appVersionStorage struct{}

func (s *appVersionStorage) collection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	coll := conn.Collection(appVersionsCollectionName)
	err = coll.EnsureIndex(mgo.Index{
		Key:    []string{"appname"},
		Unique: true,
	})
	return coll, err
}

func (s *appVersionStorage) UpdateVersion(ctx context.Context, appName string, vi *appTypes.AppVersionInfo, opts ...*appTypes.AppVersionWriteOptions) error {
	now := time.Now().UTC()
	uuidV4, err := uuid.NewV4()
	if err != nil {
		errors.WithMessage(err, "failed to generate uuid v4")
	}
	vi.UpdatedAt = now
	return s.baseUpdate(ctx, appName, bson.M{
		"$set": bson.M{
			fmt.Sprintf("versions.%d", vi.Version): vi,
			"updatedat":                            now,
			"updatedhash":                          uuidV4.String(),
		},
	}, opts...)
}

func (s *appVersionStorage) UpdateVersionSuccess(ctx context.Context, appName string, vi *appTypes.AppVersionInfo, opts ...*appTypes.AppVersionWriteOptions) error {
	now := time.Now().UTC()
	vi.UpdatedAt = now
	uuidV4, err := uuid.NewV4()
	if err != nil {
		errors.WithMessage(err, "failed to generate uuid v4")
	}
	return s.baseUpdate(ctx, appName, bson.M{
		"$set": bson.M{
			"lastsuccessfulversion":                vi.Version,
			"updatedat":                            now,
			"updatedhash":                          uuidV4.String(),
			fmt.Sprintf("versions.%d", vi.Version): vi,
		},
	}, opts...)
}

func (s *appVersionStorage) baseUpdate(ctx context.Context, appName string, updateQuery bson.M, opts ...*appTypes.AppVersionWriteOptions) error {
	where := bson.M{"appname": appName}

	// when receive a PreviousUpdatedHash will perform a optimistic update
	if len(opts) > 0 && opts[0].PreviousUpdatedHash != "" {
		where["updatedhash"] = opts[0].PreviousUpdatedHash
	}
	return s.baseUpdateWhere(ctx, where, updateQuery)
}

func (s *appVersionStorage) baseUpdateWhere(ctx context.Context, where, updateQuery bson.M) error {
	span := newMongoDBSpan(ctx, mongoSpanUpdate, appVersionsCollectionName)
	span.SetQueryStatement(where)

	defer span.Finish()

	coll, err := s.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.Update(where, updateQuery)
	if err == mgo.ErrNotFound {
		if _, exists := where["updatedhash"]; exists {
			span.LogKV("event", appTypes.ErrTransactionCancelledByChange.Error())
			return appTypes.ErrTransactionCancelledByChange
		}
		span.LogKV("event", appTypes.ErrNoVersionsAvailable.Error())
		return appTypes.ErrNoVersionsAvailable
	}
	span.SetError(err)
	return err
}

func (s *appVersionStorage) NewAppVersion(ctx context.Context, args appTypes.NewVersionArgs) (*appTypes.AppVersionInfo, error) {
	appVersions, err := s.AppVersions(ctx, args.App)
	if err != nil && err != appTypes.ErrNoVersionsAvailable {
		return nil, err
	}
	currentCount := appVersions.Count + 1

	now := time.Now().UTC()
	uuidV4, err := uuid.NewV4()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate uuid v4")
	}
	appVersionInfo := appTypes.AppVersionInfo{
		Description:    args.Description,
		Version:        currentCount,
		EventID:        args.EventID,
		CustomBuildTag: args.CustomBuildTag,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	query := bson.M{"appname": args.App.GetName()}
	span := newMongoDBSpan(ctx, mongoSpanUpsert, appVersionsCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	coll, err := s.collection()
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	defer coll.Close()
	_, err = coll.Upsert(query, bson.M{
		"$set": bson.M{
			"count":       appVersionInfo.Version,
			"updatedat":   time.Now().UTC(),
			"updatedhash": uuidV4.String(),
			fmt.Sprintf("versions.%d", appVersionInfo.Version): appVersionInfo,
		},
	})
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	return &appVersionInfo, nil
}

func (s *appVersionStorage) DeleteVersions(ctx context.Context, appName string, opts ...*appTypes.AppVersionWriteOptions) error {
	where := bson.M{"appname": appName}

	// when receive a PreviousUpdatedHash will perform a optimistic delete
	if len(opts) > 0 && opts[0].PreviousUpdatedHash != "" {
		where["updatedhash"] = opts[0].PreviousUpdatedHash
	}

	uuidV4, err := uuid.NewV4()
	if err != nil {
		return errors.WithMessage(err, "failed to generate uuid v4")
	}

	err = s.baseUpdate(ctx, appName, bson.M{
		"$set": bson.M{
			"versions":        map[int]appTypes.AppVersionInfo{},
			"updatedhash":     uuidV4.String(),
			"markedtoremoval": false,
		},
	}, opts...)

	if err == appTypes.ErrNoVersionsAvailable {
		return nil
	}

	return err
}

func (s *appVersionStorage) AllAppVersions(ctx context.Context, appNamesFilter ...string) ([]appTypes.AppVersions, error) {
	span := newMongoDBSpan(ctx, mongoSpanFind, appVersionsCollectionName)
	defer span.Finish()

	collection, err := storagev2.AppVersionsCollection()
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	var allAppVersions []appTypes.AppVersions
	var filter mongoBSON.M
	if len(appNamesFilter) > 0 {
		filter = mongoBSON.M{"appname": bson.M{"$in": appNamesFilter}}
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	err = cursor.All(ctx, &allAppVersions)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	return allAppVersions, nil
}

func (s *appVersionStorage) AppVersions(ctx context.Context, app appTypes.AppInterface) (appTypes.AppVersions, error) {
	query := mongoBSON.M{"appname": app.GetName()}
	span := newMongoDBSpan(ctx, mongoSpanFind, appVersionsCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	collection, err := storagev2.AppVersionsCollection()
	if err != nil {
		span.SetError(err)
		return appTypes.AppVersions{}, err
	}

	var appVersions appTypes.AppVersions
	err = collection.FindOne(ctx, query).Decode(&appVersions)
	if err == mongo.ErrNoDocuments {
		return appVersions, appTypes.ErrNoVersionsAvailable
	}
	if err != nil {
		span.SetError(err)
		return appTypes.AppVersions{}, err
	}
	return appVersions, nil
}

func (s *appVersionStorage) DeleteVersionIDs(ctx context.Context, appName string, versions []int, opts ...*appTypes.AppVersionWriteOptions) error {
	uuidV4, err := uuid.NewV4()
	if err != nil {
		return errors.WithMessage(err, "failed to generate uuid v4")
	}
	fullChange := bson.M{
		"$set": bson.M{
			"updatedat":   time.Now().UTC(),
			"updatedhash": uuidV4.String(),
		},
	}
	unset := bson.M{}
	for _, version := range versions {
		unset[fmt.Sprintf("versions.%d", version)] = ""
	}
	if len(unset) > 0 {
		fullChange["$unset"] = unset
	}
	return s.baseUpdate(ctx, appName, fullChange, opts...)
}

func (s *appVersionStorage) MarkToRemoval(ctx context.Context, appName string, opts ...*appTypes.AppVersionWriteOptions) error {
	uuidV4, err := uuid.NewV4()
	if err != nil {
		return errors.WithMessage(err, "failed to generate uuid v4")
	}
	update := bson.M{
		"$set": bson.M{
			"markedtoremoval": true,
			"updatedat":       time.Now().UTC(),
			"updatedhash":     uuidV4.String(),
		},
	}
	return s.baseUpdate(ctx, appName, update)
}

func (s *appVersionStorage) MarkVersionsToRemoval(ctx context.Context, appName string, versions []int, opts ...*appTypes.AppVersionWriteOptions) error {
	now := time.Now().UTC()
	uuidV4, err := uuid.NewV4()
	if err != nil {
		return errors.WithMessage(err, "failed to generate uuid v4")
	}

	where := bson.M{
		"appname": appName,
	}

	// when receive a PreviousUpdatedHash will perform a optimistic delete
	if len(opts) > 0 && opts[0].PreviousUpdatedHash != "" {
		where["updatedhash"] = opts[0].PreviousUpdatedHash
	}

	set := bson.M{
		"updatedat":   now,
		"updatedhash": uuidV4.String(),
	}

	for _, version := range versions {
		versionKey := fmt.Sprintf("versions.%d", version)
		where[versionKey] = bson.M{"$exists": true}
		set[versionKey+".markedtoremoval"] = true
		set[versionKey+".updatedat"] = now
	}

	update := bson.M{"$set": set}
	return s.baseUpdateWhere(ctx, where, update)
}
