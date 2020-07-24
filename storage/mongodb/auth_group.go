// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/auth"
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

func (s *authGroupStorage) List(filter []string) ([]auth.Group, error) {
	coll, err := s.collection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	bsonFilter := bson.M{}
	if filter != nil {
		bsonFilter["name"] = bson.M{"$in": filter}
	}
	var groups []auth.Group
	err = coll.Find(bsonFilter).All(&groups)
	return groups, err
}

func (s *authGroupStorage) AddRole(name, roleName, contextValue string) error {
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

func (s *authGroupStorage) RemoveRole(name, roleName, contextValue string) error {
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
