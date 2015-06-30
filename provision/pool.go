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
	Name    string `bson:"_id"`
	Teams   []string
	Public  bool
	Default bool
}

var ErrPublicDefaultPollCantHaveTeams = errors.New("Public/Default pool can't have teams.")

const poolCollection = "pool"

type AddPoolOptions struct {
	Name    string
	Public  bool
	Default bool
}

func AddPool(opts AddPoolOptions) error {
	if opts.Name == "" {
		return errors.New("Pool name is required.")
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if opts.Default {
		p, err := ListPools(bson.M{"default": true})
		if err != nil {
			return err
		}
		if len(p) > 0 {
			return errors.New("Default pool already exists.")
		}
	}
	pool := Pool{Name: opts.Name, Public: opts.Public, Default: opts.Default}
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
	if pool.Public || pool.Default {
		return ErrPublicDefaultPollCantHaveTeams
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

type PoolUpdateOptions struct {
	Name    string
	NewName string
	Public  bool
}

func PoolUpdate(params PoolUpdateOptions) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var p Pool
	err = conn.Collection(poolCollection).Find(bson.M{"_id": params.Name}).One(&p)
	if err != nil {
		return err
	}
	return conn.Collection(poolCollection).UpdateId(params.Name, bson.M{"public": params.Public})
}

// GetPoolsNames find teams by a list of team names.
func GetPoolsNames(pools []Pool) []string {
	pn := make([]string, len(pools))
	for i, p := range pools {
		pn[i] = p.Name
	}
	return pn
}
