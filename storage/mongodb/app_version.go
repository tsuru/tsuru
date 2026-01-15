// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db/storagev2"
	appTypes "github.com/tsuru/tsuru/types/app"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type appVersionStorage struct{}

func (s *appVersionStorage) UpdateVersion(ctx context.Context, appName string, vi *appTypes.AppVersionInfo, opts ...*appTypes.AppVersionWriteOptions) error {
	now := time.Now().UTC()
	uuidV4, err := uuid.NewRandom()
	if err != nil {
		errors.WithMessage(err, "failed to generate uuid v4")
	}
	vi.UpdatedAt = now
	return s.baseUpdate(ctx, appName, mongoBSON.M{
		"$set": mongoBSON.M{
			fmt.Sprintf("versions.%d", vi.Version): vi,
			"updatedat":                            now,
			"updatedhash":                          uuidV4.String(),
		},
	}, opts...)
}

func (s *appVersionStorage) UpdateVersionSuccess(ctx context.Context, appName string, vi *appTypes.AppVersionInfo, opts ...*appTypes.AppVersionWriteOptions) error {
	now := time.Now().UTC()
	vi.UpdatedAt = now
	uuidV4, err := uuid.NewRandom()
	if err != nil {
		errors.WithMessage(err, "failed to generate uuid v4")
	}
	return s.baseUpdate(ctx, appName, mongoBSON.M{
		"$set": mongoBSON.M{
			"lastsuccessfulversion":                vi.Version,
			"updatedat":                            now,
			"updatedhash":                          uuidV4.String(),
			fmt.Sprintf("versions.%d", vi.Version): vi,
		},
	}, opts...)
}

func (s *appVersionStorage) baseUpdate(ctx context.Context, appName string, updateQuery mongoBSON.M, opts ...*appTypes.AppVersionWriteOptions) error {
	where := mongoBSON.M{"appname": appName}

	// when receive a PreviousUpdatedHash will perform a optimistic update
	if len(opts) > 0 && opts[0].PreviousUpdatedHash != "" {
		where["updatedhash"] = opts[0].PreviousUpdatedHash
	}
	return s.baseUpdateWhere(ctx, where, updateQuery)
}

func (s *appVersionStorage) baseUpdateWhere(ctx context.Context, where, updateQuery mongoBSON.M) error {
	collection, err := storagev2.AppVersionsCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanUpdate, collection.Name())
	span.SetQueryStatement(where)

	defer span.Finish()

	err = collection.FindOneAndUpdate(ctx, where, updateQuery).Err()
	if err == mongo.ErrNoDocuments {
		if _, exists := where["updatedhash"]; exists {
			span.LogKV("event", appTypes.ErrTransactionCancelledByChange.Error())
			return appTypes.ErrTransactionCancelledByChange
		}
		span.LogKV("event", appTypes.ErrNoVersionsAvailable.Error())
		return appTypes.ErrNoVersionsAvailable
	}
	if err != nil {
		span.SetError(err)
		return err
	}

	return nil
}

func (s *appVersionStorage) NewAppVersion(ctx context.Context, args appTypes.NewVersionArgs) (*appTypes.AppVersionInfo, error) {
	appVersions, err := s.AppVersions(ctx, args.App)
	if err != nil && err != appTypes.ErrNoVersionsAvailable {
		return nil, err
	}
	currentCount := appVersions.Count + 1

	now := time.Now().UTC()
	uuidV4, err := uuid.NewRandom()
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

	query := mongoBSON.M{"appname": args.App.Name}

	collection, err := storagev2.AppVersionsCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanUpsert, collection.Name())
	span.SetQueryStatement(query)
	defer span.Finish()

	_, err = collection.UpdateOne(ctx, query, mongoBSON.M{
		"$set": mongoBSON.M{
			"count":       appVersionInfo.Version,
			"updatedat":   time.Now().UTC(),
			"updatedhash": uuidV4.String(),
			fmt.Sprintf("versions.%d", appVersionInfo.Version): appVersionInfo,
		},
	}, options.Update().SetUpsert(true))
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	return &appVersionInfo, nil
}

func (s *appVersionStorage) DeleteVersions(ctx context.Context, appName string, opts ...*appTypes.AppVersionWriteOptions) error {
	where := mongoBSON.M{"appname": appName}

	// when receive a PreviousUpdatedHash will perform a optimistic delete
	if len(opts) > 0 && opts[0].PreviousUpdatedHash != "" {
		where["updatedhash"] = opts[0].PreviousUpdatedHash
	}

	uuidV4, err := uuid.NewRandom()
	if err != nil {
		return errors.WithMessage(err, "failed to generate uuid v4")
	}

	err = s.baseUpdate(ctx, appName, mongoBSON.M{
		"$set": mongoBSON.M{
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
	collection, err := storagev2.AppVersionsCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	defer span.Finish()

	var allAppVersions []appTypes.AppVersions
	var filter mongoBSON.M
	if len(appNamesFilter) > 0 {
		filter = mongoBSON.M{"appname": mongoBSON.M{"$in": appNamesFilter}}
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

func (s *appVersionStorage) AppVersions(ctx context.Context, app *appTypes.App) (appTypes.AppVersions, error) {
	query := mongoBSON.M{"appname": app.Name}

	collection, err := storagev2.AppVersionsCollection()
	if err != nil {
		return appTypes.AppVersions{}, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	span.SetQueryStatement(query)
	defer span.Finish()

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
	uuidV4, err := uuid.NewRandom()
	if err != nil {
		return errors.WithMessage(err, "failed to generate uuid v4")
	}
	fullChange := mongoBSON.M{
		"$set": mongoBSON.M{
			"updatedat":   time.Now().UTC(),
			"updatedhash": uuidV4.String(),
		},
	}
	unset := mongoBSON.M{}
	for _, version := range versions {
		unset[fmt.Sprintf("versions.%d", version)] = ""
	}
	if len(unset) > 0 {
		fullChange["$unset"] = unset
	}
	return s.baseUpdate(ctx, appName, fullChange, opts...)
}

func (s *appVersionStorage) MarkToRemoval(ctx context.Context, appName string, opts ...*appTypes.AppVersionWriteOptions) error {
	uuidV4, err := uuid.NewRandom()
	if err != nil {
		return errors.WithMessage(err, "failed to generate uuid v4")
	}
	update := mongoBSON.M{
		"$set": mongoBSON.M{
			"markedtoremoval": true,
			"updatedat":       time.Now().UTC(),
			"updatedhash":     uuidV4.String(),
		},
	}
	return s.baseUpdate(ctx, appName, update)
}

func (s *appVersionStorage) MarkVersionsToRemoval(ctx context.Context, appName string, versions []int, opts ...*appTypes.AppVersionWriteOptions) error {
	now := time.Now().UTC()
	uuidV4, err := uuid.NewRandom()
	if err != nil {
		return errors.WithMessage(err, "failed to generate uuid v4")
	}

	where := mongoBSON.M{
		"appname": appName,
	}

	// when receive a PreviousUpdatedHash will perform a optimistic delete
	if len(opts) > 0 && opts[0].PreviousUpdatedHash != "" {
		where["updatedhash"] = opts[0].PreviousUpdatedHash
	}

	set := mongoBSON.M{
		"updatedat":   now,
		"updatedhash": uuidV4.String(),
	}

	for _, version := range versions {
		versionKey := fmt.Sprintf("versions.%d", version)
		where[versionKey] = mongoBSON.M{"$exists": true}
		set[versionKey+".markedtoremoval"] = true
		set[versionKey+".updatedat"] = now
	}

	update := mongoBSON.M{"$set": set}
	return s.baseUpdateWhere(ctx, where, update)
}
