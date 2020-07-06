// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/globalsign/mgo"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/router"
)

type dynamicRouter struct {
	Name   string `bson:"_id"`
	Type   string
	Config map[string]interface{} `bson:",omitempty"`
}

type dynamicRouterStorage struct{}

func (s *dynamicRouterStorage) coll(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection("dynamic_routers")
}

func (s *dynamicRouterStorage) Save(dr router.DynamicRouter) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.coll(conn).UpsertId(dr.Name, dynamicRouter(dr))
	if err != nil {
		return err
	}
	return nil
}

func (s *dynamicRouterStorage) Get(name string) (*router.DynamicRouter, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var dr dynamicRouter
	err = s.coll(conn).FindId(name).One(&dr)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, router.ErrDynamicRouterNotFound
		}
		return nil, err
	}
	result := router.DynamicRouter(dr)
	return &result, nil
}

func (s *dynamicRouterStorage) List() ([]router.DynamicRouter, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var drs []dynamicRouter
	err = s.coll(conn).Find(nil).All(&drs)
	if err != nil {
		return nil, err
	}
	result := make([]router.DynamicRouter, len(drs))
	for i := range drs {
		result[i] = router.DynamicRouter(drs[i])
	}
	return result, nil
}

func (s *dynamicRouterStorage) Remove(name string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = s.coll(conn).RemoveId(name)
	if err != nil {
		if err == mgo.ErrNotFound {
			return router.ErrDynamicRouterNotFound
		}
		return err
	}
	return nil
}
