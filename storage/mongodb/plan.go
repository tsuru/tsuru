// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	mgo "github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"

	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/app"
)

var _ app.PlanStorage = &PlanStorage{}

type PlanStorage struct{}

type plan struct {
	Name     string `bson:"_id"`
	Memory   int64
	Swap     int64
	CpuShare int
	Default  bool
}

func plansCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection("plans")
}

func (s *PlanStorage) Insert(p app.Plan) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if p.Default {
		_, err = plansCollection(conn).UpdateAll(bson.M{"default": true}, bson.M{"$unset": bson.M{"default": false}})
		if err != nil {
			return err
		}
	}
	err = plansCollection(conn).Insert(plan(p))
	if err != nil && mgo.IsDup(err) {
		return app.ErrPlanAlreadyExists
	}
	return err
}

func (s *PlanStorage) FindAll() ([]app.Plan, error) {
	return s.findByQuery(nil)
}

func (s *PlanStorage) FindDefault() (*app.Plan, error) {
	plans, err := s.findByQuery(bson.M{"default": true})
	if err != nil {
		return nil, err
	}
	if len(plans) > 1 {
		return nil, app.ErrPlanDefaultAmbiguous
	}
	if len(plans) == 0 {
		return nil, nil
	}
	return &plans[0], err
}

func (s *PlanStorage) findByQuery(query bson.M) ([]app.Plan, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var plans []plan
	err = plansCollection(conn).Find(query).All(&plans)
	if err != nil {
		return nil, err
	}
	appPlans := make([]app.Plan, len(plans))
	for i, p := range plans {
		appPlans[i] = app.Plan(p)
	}
	return appPlans, nil
}

func (s *PlanStorage) FindByName(name string) (*app.Plan, error) {
	var p plan
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = plansCollection(conn).FindId(name).One(&p)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = app.ErrPlanNotFound
		}
		return nil, err
	}
	plan := app.Plan(p)
	return &plan, nil
}

func (s *PlanStorage) Delete(p app.Plan) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = plansCollection(conn).RemoveId(p.Name)
	if err == mgo.ErrNotFound {
		return app.ErrPlanNotFound
	}
	return err
}
