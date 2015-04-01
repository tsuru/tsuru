// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"errors"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/mgo.v2/bson"
)

type Pool struct {
	Name  string `bson:"_id"`
	Teams []string
}

const poolCollection = "pool"

func AddPool(poolName string) error {
	if poolName == "" {
		return errors.New("Pool name is required.")
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	pool := Pool{Name: poolName}
	return conn.Collection(poolCollection).Insert(pool)
}

func RemovePool(poolName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Collection(poolCollection).Remove(bson.M{"_id": poolName})
}

func AddTeamsToPool(poolName string, teams []string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var pool Pool
	err = conn.Collection(poolCollection).Find(bson.M{"_id": poolName}).One(&pool)
	if err != nil {
		return err
	}
	for _, newTeam := range teams {
		for _, team := range pool.Teams {
			if newTeam == team {
				return errors.New("Team already exists in pool.")
			}
		}
	}
	return conn.Collection(poolCollection).UpdateId(poolName, bson.M{"$push": bson.M{"teams": bson.M{"$each": teams}}})
}

func RemoveTeamsFromPool(poolName string, teams []string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Collection(poolCollection).UpdateId(poolName, bson.M{"$pullAll": bson.M{"teams": teams}})
}

func ListPools(query bson.M) ([]Pool, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var pools []Pool
	err = conn.Collection(poolCollection).Find(query).All(&pools)
	if err != nil {
		return nil, err
	}
	return pools, nil
}
