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

type routerTemplate struct {
	Name   string `bson:"_id"`
	Type   string
	Config map[string]interface{} `bson:",omitempty"`
}

type routerTemplateStorage struct{}

func (s *routerTemplateStorage) coll(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection("router_templates")
}

func (s *routerTemplateStorage) Save(rt router.RouterTemplate) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.coll(conn).UpsertId(rt.Name, routerTemplate(rt))
	if err != nil {
		return err
	}
	return nil
}

func (s *routerTemplateStorage) Get(name string) (*router.RouterTemplate, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var rt routerTemplate
	err = s.coll(conn).FindId(name).One(&rt)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, router.ErrRouterTemplateNotFound
		}
		return nil, err
	}
	result := router.RouterTemplate(rt)
	return &result, nil
}

func (s *routerTemplateStorage) List() ([]router.RouterTemplate, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var rts []routerTemplate
	err = s.coll(conn).Find(nil).All(&rts)
	if err != nil {
		return nil, err
	}
	result := make([]router.RouterTemplate, len(rts))
	for i := range rts {
		result[i] = router.RouterTemplate(rts[i])
	}
	return result, nil
}

func (s *routerTemplateStorage) Remove(name string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = s.coll(conn).RemoveId(name)
	if err != nil {
		if err == mgo.ErrNotFound {
			return router.ErrRouterTemplateNotFound
		}
		return err
	}
	return nil
}
