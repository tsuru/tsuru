// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrPublicDefaultPoolCantHaveTeams = errors.New("Public/Default pool can't have teams.")
	ErrDefaultPoolAlreadyExists       = errors.New("Default pool already exists.")
	ErrPoolNameIsRequired             = errors.New("Pool name is required.")
	ErrPoolNotFound                   = errors.New("Pool does not exist.")
	ErrPoolHasNoTeam                  = errors.New("no team found for pool")
)

type Pool struct {
	Name        string `bson:"_id"`
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

func (p *Pool) GetTeams() ([]string, error) {
	constraints, err := getConstraintsForPool(p.Name, "team")
	if err != nil {
		return nil, err
	}
	if c, ok := constraints["team"]; ok {
		if c.WhiteList == true {
			return c.Values, nil
		}
	}
	return nil, ErrPoolHasNoTeam
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
		return ErrPublicDefaultPoolCantHaveTeams
	}
	for _, newTeam := range teams {
		check, err := checkPoolExactConstraint(poolName, "team", newTeam)
		if err != nil {
			return err
		}
		if check {
			return errors.New("Team already exists in pool.")
		}
	}
	return appendPoolConstraint(poolName, "team", teams...)
}

func RemoveTeamsFromPool(poolName string, teams []string) error {
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
	return removePoolConstraint(poolName, "team", teams...)
}

func ListPossiblePools(teams []string) ([]Pool, error) {
	teamPools, err := getPoolsSatisfyConstraints("team", teams...)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, p := range teamPools {
		names = append(names, p.Name)
	}
	query := bson.M{
		"$and": []bson.M{
			{"$or": []bson.M{{"public": true}, {"default": true}}},
			{"name": bson.M{"$nin": names}},
		},
	}
	publicPools, err := listPools(query)
	if err != nil {
		return nil, err
	}
	return append(publicPools, teamPools...), nil
}

func ListPoolsForTeam(team string) ([]Pool, error) {
	return getPoolsSatisfyConstraints("team", team)
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

type constraint struct {
	PoolExpr  string
	Field     string
	Values    []string
	WhiteList bool
}

func (c *constraint) check(v string) bool {
	for _, r := range c.Values {
		if match, _ := regexp.MatchString(strings.Replace(r, "*", ".*", -1), v); match {
			return c.WhiteList
		}
	}
	return !c.WhiteList
}

func (c *constraint) String() string {
	op := "!="
	if c.WhiteList {
		op = "="
	}
	return fmt.Sprintf("PoolExpr: %s - %s%s%s", c.PoolExpr, c.Field, op, strings.Join(c.Values, ","))
}

type constraintList []*constraint

func (l constraintList) Len() int      { return len(l) }
func (l constraintList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l constraintList) Less(i, j int) bool {
	lenI, lenJ := len(l[i].PoolExpr), len(l[j].PoolExpr)
	if lenI == lenJ {
		return strings.Count(l[i].PoolExpr, "*") < strings.Count(l[j].PoolExpr, "*")
	}
	return lenI > lenJ
}

func (l constraintList) String() string {
	s := make([]string, len(l))
	for i := range l {
		s[i] = l[i].String()
	}
	return strings.Join(s, "\n")
}

func SetPoolConstraints(poolExpr string, constraints ...string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	for _, c := range constraints {
		op := "="
		if strings.Contains(c, "!=") {
			op = "!="
		}
		parts := strings.SplitN(c, op, 2)
		constraint := &constraint{
			PoolExpr:  poolExpr,
			Field:     parts[0],
			Values:    strings.Split(parts[1], ","),
			WhiteList: op == "=",
		}
		_, err := conn.PoolsConstraints().Upsert(bson.M{"poolexpr": poolExpr, "field": parts[0]}, constraint)
		if err != nil {
			return err
		}
	}
	return nil
}

func appendPoolConstraint(poolExpr string, field string, values ...string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.PoolsConstraints().Upsert(
		bson.M{"poolexpr": poolExpr, "field": field},
		bson.M{"$pushAll": bson.M{"values": values},
			"$setOnInsert": bson.M{"whitelist": true},
		},
	)
	return err
}

func removePoolConstraint(poolExpr string, field string, values ...string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.PoolsConstraints().Update(bson.M{"poolexpr": poolExpr, "field": field}, bson.M{"$pullAll": bson.M{"values": values}})
}

func checkPoolExactConstraint(pool, field, value string) (bool, error) {
	conn, err := db.Conn()
	if err != nil {
		return false, err
	}
	defer conn.Close()
	var constraint *constraint
	err = conn.PoolsConstraints().Find(bson.M{"poolexpr": pool, "field": field, "whitelist": true}).One(&constraint)
	if err != nil {
		if err == mgo.ErrNotFound {
			return false, nil
		}
		return false, err
	}
	return constraint.check(value), nil
}

func checkPoolConstraint(pool, field, value string) (bool, error) {
	constraints, err := getConstraintsForPool(pool, field)
	if err != nil {
		return false, err
	}
	if c, ok := constraints[field]; ok {
		if !c.check(value) {
			return false, nil
		}
	}
	return true, nil
}

func getPoolsSatisfyConstraints(field string, values ...string) ([]Pool, error) {
	pools, err := listPools(nil)
	if err != nil {
		return nil, err
	}
	var satisfying []Pool
	for _, p := range pools {
		constraints, err := getConstraintsForPool(p.Name, field)
		if err != nil {
			return nil, err
		}
		c, ok := constraints[field]
		if !ok || c.PoolExpr != p.Name {
			continue
		}
		for _, v := range values {
			if !c.check(v) {
				continue
			}
		}
		satisfying = append(satisfying, p)
	}
	return satisfying, nil
}

func getConstraintsForPool(pool string, fields ...string) (map[string]*constraint, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var constraints []*constraint
	var query bson.M
	if len(fields) > 0 {
		query = bson.M{"field": bson.M{"$in": fields}}
	}
	err = conn.PoolsConstraints().Find(query).All(&constraints)
	if err != nil {
		return nil, err
	}
	var matches []*constraint
	for _, c := range constraints {
		match, err := regexp.MatchString(strings.Replace(c.PoolExpr, "*", ".*", -1), pool)
		if err != nil {
			return nil, err
		}
		if match {
			matches = append(matches, c)
		}
	}
	sort.Sort(constraintList(matches))
	merged := make(map[string]*constraint)
	for i := range matches {
		if _, ok := merged[matches[i].Field]; !ok {
			merged[matches[i].Field] = matches[i]
		}
	}
	return merged, nil
}
