// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
)

// Plan represents a service plan
type Plan struct {
	Name        string
	ServiceName string
}

// CreatePlan store a new plan into database.
func CreatePlan(p *Plan) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Plans().Insert(p)
}

// GetPlanByName get a plan by name.
func GetPlanByName(name string) (*Plan, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var p Plan
	err = conn.Plans().Find(bson.M{"name": name}).One(&p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func DeletePlan(p *Plan) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Plans().Remove(bson.M{"name": p.Name})
}
