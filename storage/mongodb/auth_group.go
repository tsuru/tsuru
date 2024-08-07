// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/auth"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var errAuthGroupNameEmpty = errors.New("group name cannot be empty")

type authGroupStorage struct{}

func (s *authGroupStorage) List(ctx context.Context, filter []string) ([]auth.Group, error) {
	collection, err := storagev2.AuthGroupsCollection()
	if err != nil {
		return nil, err
	}

	bsonFilter := mongoBSON.M{}
	if filter != nil {
		bsonFilter["name"] = mongoBSON.M{"$in": filter}
	}

	cursor, err := collection.Find(ctx, bsonFilter)
	if err != nil {
		return nil, err
	}

	var groups []auth.Group
	err = cursor.All(ctx, &groups)
	if err != nil {
		return nil, err
	}

	return groups, nil
}

func (s *authGroupStorage) AddRole(ctx context.Context, name, roleName, contextValue string) error {
	if name == "" {
		return errAuthGroupNameEmpty
	}
	collection, err := storagev2.AuthGroupsCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": name}, mongoBSON.M{
		"$addToSet": mongoBSON.M{
			"roles": roleToBson(auth.RoleInstance{Name: roleName, ContextValue: contextValue}),
		},
	}, options.Update().SetUpsert(true))
	return err
}

func (s *authGroupStorage) RemoveRole(ctx context.Context, name, roleName, contextValue string) error {
	if name == "" {
		return errAuthGroupNameEmpty
	}
	collection, err := storagev2.AuthGroupsCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": name}, mongoBSON.M{
		"$pullAll": mongoBSON.M{
			"roles": []mongoBSON.D{roleToBson(auth.RoleInstance{Name: roleName, ContextValue: contextValue})},
		},
	})
	return err
}

func roleToBson(ri auth.RoleInstance) mongoBSON.D {
	// Order matters in $addToSet, that's why bson.D is used instead
	// of bson.M.
	return mongoBSON.D([]primitive.E{
		{Key: "name", Value: ri.Name},
		{Key: "contextvalue", Value: ri.ContextValue},
	})
}
