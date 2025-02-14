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

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db/storagev2"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrInvalidConstraintType = errors.Errorf("invalid constraint type. Valid types are: %s", validConstraintTypes)
	validConstraintTypes     = []PoolConstraintType{ConstraintTypeTeam, ConstraintTypeService, ConstraintTypeRouter, ConstraintTypePlan, ConstraintTypeVolumePlan, ConstraintTypeCertIssuer} // new pool constraint type for cert-issuers
)

type PoolConstraintType string

const (
	ConstraintTypeTeam       = PoolConstraintType("team")
	ConstraintTypeRouter     = PoolConstraintType("router")
	ConstraintTypeService    = PoolConstraintType("service")
	ConstraintTypePlan       = PoolConstraintType("plan")
	ConstraintTypeVolumePlan = PoolConstraintType("volume-plan")
	ConstraintTypeCertIssuer = PoolConstraintType("cert-issuer") // new pool constraint type for cert-issuers
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
	Field     PoolConstraintType
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

func SetPoolConstraint(ctx context.Context, c *PoolConstraint) error {
	collection, err := storagev2.PoolConstraintsCollection()
	if err != nil {
		return err
	}
	isValid := validateConstraintType(c.Field)
	if !isValid {
		return ErrInvalidConstraintType
	}
	if len(c.Values) == 0 || (len(c.Values) == 1 && c.Values[0] == "") {
		result, errRem := collection.DeleteMany(ctx, mongoBSON.M{"poolexpr": c.PoolExpr, "field": c.Field})
		if errRem != mongo.ErrNoDocuments {
			return errRem
		}

		if result.DeletedCount > 0 {
			return nil
		}
	}

	opts := options.Update().SetUpsert(true)
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"poolexpr": c.PoolExpr, "field": c.Field}, mongoBSON.M{"$set": c}, opts)
	return err
}

func AppendPoolConstraint(ctx context.Context, c *PoolConstraint) error {
	isValid := validateConstraintType(c.Field)
	if !isValid {
		return ErrInvalidConstraintType
	}
	return appendPoolConstraint(ctx, c.PoolExpr, c.Field, c.Values...)
}

func validateConstraintType(c PoolConstraintType) bool {
	for _, v := range validConstraintTypes {
		if c == v {
			return true
		}
	}
	return false
}

func appendPoolConstraint(ctx context.Context, poolExpr string, field PoolConstraintType, values ...string) error {
	collection, err := storagev2.PoolConstraintsCollection()
	if err != nil {
		return err
	}

	opts := options.Update().SetUpsert(true)
	_, err = collection.UpdateOne(ctx,
		mongoBSON.M{"poolexpr": poolExpr, "field": field},
		mongoBSON.M{"$addToSet": mongoBSON.M{"values": mongoBSON.M{"$each": values}}},
		opts,
	)
	return err
}

func removePoolConstraint(ctx context.Context, poolExpr string, field PoolConstraintType, values ...string) error {
	collection, err := storagev2.PoolConstraintsCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"poolexpr": poolExpr, "field": field}, mongoBSON.M{"$pullAll": mongoBSON.M{"values": values}})
	return err
}

func getPoolsSatisfyConstraints(ctx context.Context, exactCheck bool, field PoolConstraintType, values ...string) ([]Pool, error) {
	pools, err := listPools(ctx, nil)
	if err != nil {
		return nil, err
	}
	var satisfying []Pool
	for _, p := range pools {
		checked := false
		constraints, err := getConstraintsForPool(ctx, p.Name, field)
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

func getConstraintsForPool(ctx context.Context, pool string, fields ...PoolConstraintType) (map[PoolConstraintType]*PoolConstraint, error) {
	var query mongoBSON.M
	if len(fields) > 0 {
		query = mongoBSON.M{"field": mongoBSON.M{"$in": fields}}
	}
	constraints, err := ListPoolsConstraints(ctx, query)
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
	merged := make(map[PoolConstraintType]*PoolConstraint)
	for i := range matches {
		if _, ok := merged[matches[i].Field]; !ok {
			merged[matches[i].Field] = matches[i]
		}
	}
	return merged, nil
}

func getExactConstraintForPool(ctx context.Context, pool string, field PoolConstraintType) (*PoolConstraint, error) {
	constraints, err := ListPoolsConstraints(ctx, mongoBSON.M{"poolexpr": pool, "field": field})
	if err != nil {
		return nil, err
	}
	if len(constraints) == 0 {
		return nil, nil
	}
	return constraints[0], nil
}

func ListPoolsConstraints(ctx context.Context, query mongoBSON.M) ([]*PoolConstraint, error) {
	collection, err := storagev2.PoolConstraintsCollection()
	if err != nil {
		return nil, err
	}

	if query == nil {
		query = mongoBSON.M{}
	}

	constraints := []*PoolConstraint{}
	cursor, err := collection.Find(ctx, query)
	if err != nil {
		return nil, err
	}

	err = cursor.All(ctx, &constraints)
	if err != nil {
		return nil, err
	}
	return constraints, nil
}

func ToConstraintType(value string) (PoolConstraintType, error) {
	if !validateConstraintType(PoolConstraintType(value)) {
		return "", ErrInvalidConstraintType
	}
	return PoolConstraintType(value), nil
}
