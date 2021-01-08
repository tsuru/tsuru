// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"

	mgo "github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
)

var (
	ErrInvalidConstraintType = errors.Errorf("invalid constraint type. Valid types are: %s", validConstraintTypes)
	validConstraintTypes     = []poolConstraintType{ConstraintTypeTeam, ConstraintTypeService, ConstraintTypeRouter, ConstraintTypePlan}
)

type poolConstraintType string

const (
	ConstraintTypeTeam       = poolConstraintType("team")
	ConstraintTypeRouter     = poolConstraintType("router")
	ConstraintTypeService    = poolConstraintType("service")
	ConstraintTypePlan       = poolConstraintType("plan")
	ConstraintTypeVolumePlan = poolConstraintType("volume-plan")
)

type regexpCache struct {
	m sync.Map
}

var rCache = regexpCache{}

func (c *regexpCache) Lookup(pattern string) (*regexp.Regexp, error) {
	cached, _ := c.m.Load(pattern)
	if cached == nil {
		var err error
		cached, err = regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		c.m.Store(pattern, cached)
	}
	return cached.(*regexp.Regexp), nil
}

func (c *regexpCache) MatchString(pattern, value string) (bool, error) {
	r, err := c.Lookup(pattern)
	if err != nil {
		return false, err
	}
	return r.MatchString(value), nil
}

type PoolConstraint struct {
	PoolExpr  string
	Field     poolConstraintType
	Values    []string
	Blacklist bool
}

func (c *PoolConstraint) checkExact(v string) bool {
	if c == nil {
		return false
	}
	for _, r := range c.Values {
		if r == v {
			return !c.Blacklist
		}
	}
	return c.Blacklist
}

func (c *PoolConstraint) check(v string) bool {
	if c == nil {
		return false
	}
	for _, r := range c.Values {
		pattern := exprAsGlobPattern(r)
		if match, _ := rCache.MatchString(pattern, v); match {
			return !c.Blacklist
		}
	}
	return c.Blacklist
}

func (c *PoolConstraint) AllowsAll() bool {
	if c == nil || c.Blacklist {
		return false
	}
	return c.checkExact("*")
}

type constraintList []*PoolConstraint

func (l constraintList) Len() int      { return len(l) }
func (l constraintList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l constraintList) Less(i, j int) bool {
	lenI, lenJ := len(l[i].PoolExpr), len(l[j].PoolExpr)
	if lenI == lenJ {
		return strings.Count(l[i].PoolExpr, "*") < strings.Count(l[j].PoolExpr, "*")
	}
	return lenI > lenJ
}

func SetPoolConstraint(c *PoolConstraint) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	isValid := validateConstraintType(c.Field)
	if !isValid {
		return ErrInvalidConstraintType
	}
	if len(c.Values) == 0 || (len(c.Values) == 1 && c.Values[0] == "") {
		errRem := conn.PoolsConstraints().Remove(bson.M{"poolexpr": c.PoolExpr, "field": c.Field})
		if errRem != mgo.ErrNotFound {
			return errRem
		}
		return nil
	}
	_, err = conn.PoolsConstraints().Upsert(bson.M{"poolexpr": c.PoolExpr, "field": c.Field}, c)
	return err
}

func AppendPoolConstraint(c *PoolConstraint) error {
	isValid := validateConstraintType(c.Field)
	if !isValid {
		return ErrInvalidConstraintType
	}
	return appendPoolConstraint(c.PoolExpr, c.Field, c.Values...)
}

func validateConstraintType(c poolConstraintType) bool {
	for _, v := range validConstraintTypes {
		if c == v {
			return true
		}
	}
	return false
}

func appendPoolConstraint(poolExpr string, field poolConstraintType, values ...string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.PoolsConstraints().Upsert(
		bson.M{"poolexpr": poolExpr, "field": field},
		bson.M{"$addToSet": bson.M{"values": bson.M{"$each": values}}},
	)
	return err
}

func removePoolConstraint(poolExpr string, field poolConstraintType, values ...string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.PoolsConstraints().Update(bson.M{"poolexpr": poolExpr, "field": field}, bson.M{"$pullAll": bson.M{"values": values}})
}

func getPoolsSatisfyConstraints(ctx context.Context, exactCheck bool, field poolConstraintType, values ...string) ([]Pool, error) {
	pools, err := listPools(ctx, nil)
	if err != nil {
		return nil, err
	}
	var satisfying []Pool
	for _, p := range pools {
		checked := false
		constraints, err := getConstraintsForPool(p.Name, field)
		if err != nil {
			return nil, err
		}
		c, ok := constraints[field]
		if !ok || c.PoolExpr != p.Name {
			continue
		}
		for _, v := range values {
			if exactCheck && !c.checkExact(v) {
				continue
			}
			if !exactCheck && !c.check(v) {
				continue
			}
			checked = true
		}
		if checked || len(values) == 0 {
			satisfying = append(satisfying, p)
		}
	}
	return satisfying, nil
}

func getConstraintsForPool(pool string, fields ...poolConstraintType) (map[poolConstraintType]*PoolConstraint, error) {
	var query bson.M
	if len(fields) > 0 {
		query = bson.M{"field": bson.M{"$in": fields}}
	}
	constraints, err := ListPoolsConstraints(query)
	if err != nil {
		return nil, err
	}
	var matches []*PoolConstraint
	for _, c := range constraints {
		pattern := exprAsGlobPattern(c.PoolExpr)
		match, err := rCache.MatchString(pattern, pool)
		if err != nil {
			return nil, err
		}
		if match {
			matches = append(matches, c)
		}
	}
	sort.Sort(constraintList(matches))
	merged := make(map[poolConstraintType]*PoolConstraint)
	for i := range matches {
		if _, ok := merged[matches[i].Field]; !ok {
			merged[matches[i].Field] = matches[i]
		}
	}
	return merged, nil
}

func getExactConstraintForPool(pool string, field poolConstraintType) (*PoolConstraint, error) {
	constraints, err := ListPoolsConstraints(bson.M{"poolexpr": pool, "field": field})
	if err != nil {
		return nil, err
	}
	if len(constraints) == 0 {
		return nil, nil
	}
	return constraints[0], nil
}

func ListPoolsConstraints(query bson.M) ([]*PoolConstraint, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	constraints := []*PoolConstraint{}
	err = conn.PoolsConstraints().Find(query).All(&constraints)
	if err != nil {
		return nil, err
	}
	return constraints, nil
}

func ToConstraintType(value string) (poolConstraintType, error) {
	if !validateConstraintType(poolConstraintType(value)) {
		return "", ErrInvalidConstraintType
	}
	return poolConstraintType(value), nil
}
