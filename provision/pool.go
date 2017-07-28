// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/validation"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrPublicDefaultPoolCantHaveTeams = errors.New("Public/Default pool can't have teams.")
	ErrDefaultPoolAlreadyExists       = errors.New("Default pool already exists.")
	ErrPoolNameIsRequired             = errors.New("Pool name is required.")
	ErrPoolNotFound                   = errors.New("Pool does not exist.")
	ErrPoolAlreadyExists              = errors.New("Pool already exists.")
	ErrPoolHasNoTeam                  = errors.New("no team found for pool")
	ErrPoolHasNoRouter                = errors.New("no router found for pool")

	ErrInvalidConstraintType = errors.Errorf("invalid constraint type. Valid types are: %s", strings.Join(validConstraintTypes, ","))
	validConstraintTypes     = []string{"team", "router"}
)

type Pool struct {
	Name        string `bson:"_id"`
	Default     bool
	Provisioner string
	Builder     string
}

type AddPoolOptions struct {
	Name        string
	Public      bool
	Default     bool
	Force       bool
	Provisioner string
	Builder     string
}

type UpdatePoolOptions struct {
	Default     *bool
	Public      *bool
	Force       bool
	Provisioner string
	Builder     string
}

func (p *Pool) GetProvisioner() (Provisioner, error) {
	if p.Provisioner != "" {
		return Get(p.Provisioner)
	}
	return GetDefault()
}

func (p *Pool) GetTeams() ([]string, error) {
	allowedValues, err := p.allowedValues()
	if err != nil {
		return nil, err
	}
	if c := allowedValues["team"]; len(c) > 0 {
		return c, nil
	}
	return nil, ErrPoolHasNoTeam
}

func (p *Pool) GetRouters() ([]string, error) {
	allowedValues, err := p.allowedValues()
	if err != nil {
		return nil, err
	}
	if c := allowedValues["router"]; len(c) > 0 {
		return c, nil
	}
	return nil, ErrPoolHasNoRouter
}

func (p *Pool) GetDefaultRouter() (string, error) {
	constraints, err := getConstraintsForPool(p.Name, "router")
	if err != nil {
		return "", err
	}
	constraint := constraints["router"]
	if constraint == nil || len(constraint.Values) == 0 {
		return router.Default()
	}
	if constraint.Blacklist || strings.Contains(constraint.Values[0], "*") {
		var allowed map[string][]string
		allowed, err = p.allowedValues()
		if err != nil {
			return "", err
		}
		if len(allowed["router"]) == 1 {
			return allowed["router"][0], nil
		}
		return router.Default()
	}
	routers, err := routersNames()
	if err != nil {
		return "", err
	}
	for _, r := range routers {
		if constraint.Values[0] == r {
			return r, nil
		}
	}
	return router.Default()
}

func (p *Pool) allowedValues() (map[string][]string, error) {
	teams, err := teamsNames()
	if err != nil {
		return nil, err
	}
	routers, err := routersNames()
	if err != nil {
		return nil, err
	}
	resolved := map[string][]string{
		"router": routers,
		"team":   teams,
	}
	constraints, err := getConstraintsForPool(p.Name, "team", "router")
	if err != nil {
		return nil, err
	}
	for k, v := range constraints {
		var names []string
		switch k {
		case "team":
			names = teams
		case "router":
			names = routers
		}
		var validNames []string
		for _, n := range names {
			if v.check(n) {
				validNames = append(validNames, n)
			}
		}
		resolved[k] = validNames
	}
	return resolved, nil
}

func routersNames() ([]string, error) {
	routers, err := router.List()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, r := range routers {
		names = append(names, r.Name)
	}
	return names, nil
}

func teamsNames() ([]string, error) {
	teams, err := auth.ListTeams()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, t := range teams {
		names = append(names, t.Name)
	}
	return names, nil
}

func (p *Pool) MarshalJSON() ([]byte, error) {
	teams, err := getExactConstraintForPool(p.Name, "team")
	if err != nil {
		return nil, err
	}
	resolvedConstraints, err := p.allowedValues()
	if err != nil {
		return nil, err
	}
	result := make(map[string]interface{})
	result["name"] = p.Name
	result["public"] = teams.AllowsAll()
	result["default"] = p.Default
	result["provisioner"] = p.Provisioner
	result["teams"] = resolvedConstraints["team"]
	result["allowed"] = resolvedConstraints
	return json.Marshal(&result)
}

func (p *Pool) validate() error {
	if p.Name == "" {
		return ErrPoolNameIsRequired
	}
	if !validation.ValidateName(p.Name) {
		msg := "Invalid pool name, pool name should have at most 63 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return &tsuruErrors.ValidationError{Message: msg}
	}
	return nil
}

func AddPool(opts AddPoolOptions) error {
	pool := Pool{Name: opts.Name, Default: opts.Default, Provisioner: opts.Provisioner}
	if err := pool.validate(); err != nil {
		return err
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
	err = conn.Pools().Insert(pool)
	if err != nil {
		if mgo.IsDup(err) {
			return ErrPoolAlreadyExists
		}
		return err
	}
	if opts.Public || opts.Default {
		return SetPoolConstraint(&PoolConstraint{PoolExpr: opts.Name, Field: "team", Values: []string{"*"}})
	}
	return nil
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
	teamConstraint, err := getExactConstraintForPool(poolName, "team")
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	if teamConstraint != nil && teamConstraint.Blacklist {
		return errors.New("Unable to add teams to blacklist constraint")
	}
	if teamConstraint.AllowsAll() || pool.Default {
		return ErrPublicDefaultPoolCantHaveTeams
	}
	for _, newTeam := range teams {
		if teamConstraint.check(newTeam) {
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
	constraint, err := getExactConstraintForPool(poolName, "team")
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	if constraint != nil && constraint.Blacklist {
		return errors.New("Unable to remove teams from blacklist constraint")
	}
	return removePoolConstraint(poolName, "team", teams...)
}

func ListPools(names ...string) ([]Pool, error) {
	return listPools(bson.M{"_id": bson.M{"$in": names}})
}

func ListAllPools() ([]Pool, error) {
	return listPools(nil)
}

func ListPossiblePools(teams []string) ([]Pool, error) {
	return getPoolsSatisfyConstraints(false, "team", teams...)
}

func ListPoolsForTeam(team string) ([]Pool, error) {
	return getPoolsSatisfyConstraints(true, "team", team)
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
	_, err = GetPoolByName(name)
	if err != nil {
		return err
	}
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
	if (opts.Public != nil && *opts.Public) || (opts.Default != nil && *opts.Default) {
		errConstraint := SetPoolConstraint(&PoolConstraint{PoolExpr: name, Field: "team", Values: []string{"*"}})
		if errConstraint != nil {
			return err
		}
	}
	if (opts.Public != nil && !*opts.Public) || (opts.Default != nil && !*opts.Default) {
		errConstraint := removePoolConstraint(name, "team", "*")
		if errConstraint != nil {
			return err
		}
	}
	if opts.Provisioner != "" {
		query["provisioner"] = opts.Provisioner
	}
	if len(query) == 0 {
		return nil
	}
	err = conn.Pools().UpdateId(name, bson.M{"$set": query})
	if err == mgo.ErrNotFound {
		return ErrPoolNotFound
	}
	return err
}

func exprAsGlobPattern(expr string) string {
	parts := strings.Split(expr, "*")
	for i := range parts {
		parts[i] = regexp.QuoteMeta(parts[i])
	}
	return fmt.Sprintf("^%s$", strings.Join(parts, ".*"))
}

type PoolConstraint struct {
	PoolExpr  string
	Field     string
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
		if match, _ := regexp.MatchString(pattern, v); match {
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
	isValid := false
	for _, v := range validConstraintTypes {
		if c.Field == v {
			isValid = true
			break
		}
	}
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
	return appendPoolConstraint(c.PoolExpr, c.Field, c.Values...)
}

func appendPoolConstraint(poolExpr string, field string, values ...string) error {
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

func removePoolConstraint(poolExpr string, field string, values ...string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.PoolsConstraints().Update(bson.M{"poolexpr": poolExpr, "field": field}, bson.M{"$pullAll": bson.M{"values": values}})
}

func getPoolsSatisfyConstraints(exactCheck bool, field string, values ...string) ([]Pool, error) {
	pools, err := listPools(nil)
	if err != nil {
		return nil, err
	}
	var satisfying []Pool
loop:
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
			if exactCheck && !c.checkExact(v) {
				continue loop
			}
			if !exactCheck && !c.check(v) {
				continue loop
			}
		}
		satisfying = append(satisfying, p)
	}
	return satisfying, nil
}

func getConstraintsForPool(pool string, fields ...string) (map[string]*PoolConstraint, error) {
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
		match, err := regexp.MatchString(pattern, pool)
		if err != nil {
			return nil, err
		}
		if match {
			matches = append(matches, c)
		}
	}
	sort.Sort(constraintList(matches))
	merged := make(map[string]*PoolConstraint)
	for i := range matches {
		if _, ok := merged[matches[i].Field]; !ok {
			merged[matches[i].Field] = matches[i]
		}
	}
	return merged, nil
}

func getExactConstraintForPool(pool, field string) (*PoolConstraint, error) {
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
