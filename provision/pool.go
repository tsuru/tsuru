// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"errors"

	"github.com/tsuru/tsuru/db"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrPublicDefaultPollCantHaveTeams = errors.New("Public/Default pool can't have teams.")
	ErrDefaultPoolAlreadyExists       = errors.New("Default pool already exists.")
	ErrPoolNameIsRequired             = errors.New("Pool name is required.")
	ErrPoolNotFound                   = errors.New("Pool does not exist.")
)

type Pool struct {
	Name        string `bson:"_id"`
	Teams       []string
	Public      bool
	Default     bool
	Provisioner string
}

type AddPoolOptions struct {
	Name        string
	Public      bool
	Default     bool
	Force       bool
	Provisioner string
}

type UpdatePoolOptions struct {
	Default     *bool
	Public      *bool
	Force       bool
	Provisioner string
}

func (p *Pool) GetProvisioner() (Provisioner, error) {
	if p.Provisioner != "" {
		return Get(p.Provisioner)
	}
	return GetDefault()
}

func AddPool(opts AddPoolOptions) error {
	if opts.Name == "" {
		return ErrPoolNameIsRequired
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if opts.Default {
		err = changeDefaultPool(opts.Force)
		if err != nil {
			return err
		}
	}
	pool := Pool{Name: opts.Name, Public: opts.Public, Default: opts.Default, Provisioner: opts.Provisioner}
	return conn.Pools().Insert(pool)
}

func changeDefaultPool(force bool) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	p, err := listPools(bson.M{"default": true})
	if err != nil {
		return err
	}
	if len(p) > 0 {
		if !force {
			return ErrDefaultPoolAlreadyExists
		}
		return conn.Pools().UpdateId(p[0].Name, bson.M{"$set": bson.M{"default": false}})
	}
	return nil
}

func RemovePool(poolName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Pools().Remove(bson.M{"_id": poolName})
	if err == mgo.ErrNotFound {
		return ErrPoolNotFound
	}
	return err
}

func AddTeamsToPool(poolName string, teams []string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var pool Pool
	err = conn.Pools().Find(bson.M{"_id": poolName}).One(&pool)
	if err == mgo.ErrNotFound {
		return ErrPoolNotFound
	}
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
	return conn.Pools().UpdateId(poolName, bson.M{"$push": bson.M{"teams": bson.M{"$each": teams}}})
}

func RemoveTeamsFromPool(poolName string, teams []string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Pools().UpdateId(poolName, bson.M{"$pullAll": bson.M{"teams": teams}})
	if err == mgo.ErrNotFound {
		return ErrPoolNotFound
	}
	return err
}

func ListPossiblePools(teams []string) ([]Pool, error) {
	query := bson.M{}
	if teams != nil {
		filter := bson.M{
			"default": false,
			"public":  false,
			"teams":   bson.M{"$in": teams},
		}
		query["$or"] = []bson.M{{"public": true}, {"default": true}, filter}
	}
	return listPools(query)
}

func ListPoolsForTeam(team string) ([]Pool, error) {
	return listPools(bson.M{"teams": team})
}

func listPools(query bson.M) ([]Pool, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	pools := []Pool{}
	err = conn.Pools().Find(query).All(&pools)
	if err != nil {
		return nil, err
	}
	return pools, nil
}

// GetPoolByName finds a pool by name
func GetPoolByName(name string) (*Pool, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var p Pool
	err = conn.Pools().FindId(name).One(&p)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrPoolNotFound
		}
		return nil, err
	}
	return &p, nil
}

func GetDefaultPool() (*Pool, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var pool Pool
	err = conn.Pools().Find(bson.M{"default": true}).One(&pool)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrPoolNotFound
		}
		return nil, err
	}
	return &pool, nil
}

func PoolUpdate(name string, opts UpdatePoolOptions) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if opts.Default != nil && *opts.Default {
		err = changeDefaultPool(opts.Force)
		if err != nil {
			return err
		}
	}
	query := bson.M{}
	if opts.Default != nil {
		query["default"] = *opts.Default
	}
	if opts.Public != nil {
		query["public"] = *opts.Public
	}
	if opts.Provisioner != "" {
		query["provisioner"] = opts.Provisioner
	}
	err = conn.Pools().UpdateId(name, bson.M{"$set": query})
	if err == mgo.ErrNotFound {
		return ErrPoolNotFound
	}
	return err
}
