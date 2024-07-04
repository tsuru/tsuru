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
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/auth"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

const (
	authGroupCollection = "auth_groups"
)

var errAuthGroupNameEmpty = errors.New("group name cannot be empty")

type authGroupStorage struct{}

func (s *authGroupStorage) collection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	coll := conn.Collection(authGroupCollection)
	err = coll.EnsureIndex(mgo.Index{
		Key:    []string{"name"},
		Unique: true,
	})
	return coll, err
}

func (s *authGroupStorage) List(ctx context.Context, filter []string) ([]auth.Group, error) {
	collection, err := storagev2.Collection(authGroupCollection)
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
	coll, err := s.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.Upsert(bson.M{"name": name}, bson.M{
		"$addToSet": bson.M{
			"roles": roleToBson(auth.RoleInstance{Name: roleName, ContextValue: contextValue}),
		},
	})
	return err
}

func (s *authGroupStorage) RemoveRole(ctx context.Context, name, roleName, contextValue string) error {
	if name == "" {
		return errAuthGroupNameEmpty
	}
	coll, err := s.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.Upsert(bson.M{"name": name}, bson.M{
		"$pullAll": bson.M{
			"roles": []bson.D{roleToBson(auth.RoleInstance{Name: roleName, ContextValue: contextValue})},
		},
	})
	return err
}

func roleToBson(ri auth.RoleInstance) bson.D {
	// Order matters in $addToSet, that's why bson.D is used instead
	// of bson.M.
	return bson.D([]bson.DocElem{
		{Name: "name", Value: ri.Name},
		{Name: "contextvalue", Value: ri.ContextValue},
	})
}
