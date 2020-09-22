// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
	provisionTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/validation"
)

var (
	ErrPublicDefaultPoolCantHaveTeams = errors.New("Public/Default pool can't have teams.")
	ErrDefaultPoolAlreadyExists       = errors.New("Default pool already exists.")
	ErrPoolNameIsRequired             = errors.New("Pool name is required.")
	ErrPoolNotFound                   = errors.New("Pool does not exist.")
	ErrPoolAlreadyExists              = errors.New("Pool already exists.")
	ErrPoolHasNoTeam                  = errors.New("no team found for pool")
	ErrPoolHasNoRouter                = errors.New("no router found for pool")
	ErrPoolHasNoService               = errors.New("no service found for pool")
	ErrPoolHasNoPlan                  = errors.New("no plan found for pool")
)

type Pool struct {
	Name        string `bson:"_id"`
	Default     bool
	Provisioner string

	ctx context.Context
}

type AddPoolOptions struct {
	Name        string
	Public      bool
	Default     bool
	Force       bool
	Provisioner string
}

type UpdatePoolOptions struct {
	Default *bool
	Public  *bool
	Force   bool
}

func (p *Pool) GetProvisioner() (provision.Provisioner, error) {
	if p.Provisioner != "" {
		return provision.Get(p.Provisioner)
	}
	return provision.GetDefault()
}

func (p *Pool) GetTeams() ([]string, error) {
	allowedValues, err := p.allowedValues()
	if err != nil {
		return nil, err
	}
	if c := allowedValues[ConstraintTypeTeam]; len(c) > 0 {
		return c, nil
	}
	return nil, ErrPoolHasNoTeam
}

func (p *Pool) GetServices() ([]string, error) {
	allowedValues, err := p.allowedValues()
	if err != nil {
		return nil, err
	}
	if c := allowedValues[ConstraintTypeService]; len(c) > 0 {
		return c, nil
	}
	return nil, ErrPoolHasNoService
}

func (p *Pool) GetRouters() ([]string, error) {
	allowedValues, err := p.allowedValues()
	if err != nil {
		return nil, err
	}
	if c := allowedValues[ConstraintTypeRouter]; len(c) > 0 {
		return c, nil
	}
	return nil, ErrPoolHasNoRouter
}

func (p *Pool) GetPlans() ([]string, error) {
	allowedValues, err := p.allowedValues()
	if err != nil {
		return nil, err
	}
	if c := allowedValues[ConstraintTypePlan]; len(c) > 0 {
		return c, nil
	}
	return nil, ErrPoolHasNoPlan
}

func (p *Pool) GetDefaultRouter() (string, error) {
	constraints, err := getConstraintsForPool(p.Name, ConstraintTypeRouter)
	if err != nil {
		return "", err
	}
	constraint := constraints[ConstraintTypeRouter]
	if constraint == nil || len(constraint.Values) == 0 {
		return router.Default(p.ctx)
	}
	if constraint.Blacklist || strings.Contains(constraint.Values[0], "*") {
		var allowed map[poolConstraintType][]string
		allowed, err = p.allowedValues()
		if err != nil {
			return "", err
		}
		if len(allowed[ConstraintTypeRouter]) == 1 {
			return allowed[ConstraintTypeRouter][0], nil
		}
		return router.Default(p.ctx)
	}
	routers, err := routersNames(p.ctx)
	if err != nil {
		return "", err
	}
	for _, r := range routers {
		if constraint.Values[0] == r {
			return r, nil
		}
	}
	return router.Default(p.ctx)
}

func (p *Pool) ValidateRouters(routers []appTypes.AppRouter) error {
	availableRouters, err := p.GetRouters()
	if err != nil {
		return &tsuruErrors.ValidationError{Message: err.Error()}
	}
	possibleMap := make(map[string]struct{}, len(availableRouters))
	for _, r := range availableRouters {
		possibleMap[r] = struct{}{}
	}
	for _, appRouter := range routers {
		_, ok := possibleMap[appRouter.Name]
		if !ok {
			msg := fmt.Sprintf("router %q is not available for pool %q. Available routers are: %q", appRouter.Name, p.Name, strings.Join(availableRouters, ", "))
			return &tsuruErrors.ValidationError{Message: msg}
		}
	}
	return nil
}

func (p *Pool) allowedValues() (map[poolConstraintType][]string, error) {
	teams, err := teamsNames(p.ctx)
	if err != nil {
		return nil, err
	}
	routers, err := routersNames(p.ctx)
	if err != nil {
		return nil, err
	}
	services, err := servicesNames(p.ctx)
	if err != nil {
		return nil, err
	}
	plans, err := plansNames(p.ctx)
	if err != nil {
		return nil, err
	}
	resolved := map[poolConstraintType][]string{
		ConstraintTypeRouter:  routers,
		ConstraintTypeService: services,
		ConstraintTypeTeam:    teams,
		ConstraintTypePlan:    plans,
	}
	constraints, err := getConstraintsForPool(p.Name, ConstraintTypeTeam, ConstraintTypeRouter, ConstraintTypeService, ConstraintTypePlan)
	if err != nil {
		return nil, err
	}
	for k, v := range constraints {
		names := resolved[k]
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

func routersNames(ctx context.Context) ([]string, error) {
	routers, err := router.List(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, r := range routers {
		names = append(names, r.Name)
	}
	return names, nil
}

func teamsNames(ctx context.Context) ([]string, error) {
	teams, err := servicemanager.Team.List(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, t := range teams {
		names = append(names, t.Name)
	}
	return names, nil
}

func servicesNames(ctx context.Context) ([]string, error) {
	services, err := service.GetServices(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, r := range services {
		names = append(names, r.Name)
	}
	return names, nil
}

func plansNames(ctx context.Context) ([]string, error) {
	plans, err := servicemanager.Plan.List(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, p := range plans {
		names = append(names, p.Name)
	}
	return names, nil
}

func (p *Pool) MarshalJSON() ([]byte, error) {
	teams, err := getExactConstraintForPool(p.Name, ConstraintTypeTeam)
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
	result["teams"] = resolvedConstraints[ConstraintTypeTeam]
	result["allowed"] = resolvedConstraints
	return json.Marshal(&result)
}

func (p *Pool) validate() error {
	if p.Name == "" {
		return ErrPoolNameIsRequired
	}
	if !validation.ValidateName(p.Name) {
		msg := "Invalid pool name, pool name should have at most 40 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return &tsuruErrors.ValidationError{Message: msg}
	}
	return nil
}

func AddPool(ctx context.Context, opts AddPoolOptions) error {
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
		err = changeDefaultPool(ctx, opts.Force)
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
		return SetPoolConstraint(&PoolConstraint{PoolExpr: opts.Name, Field: ConstraintTypeTeam, Values: []string{"*"}})
	}
	return nil
}

func RenamePoolTeam(ctx context.Context, oldName, newName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	query := bson.M{
		"field":  "team",
		"values": oldName,
	}
	bulk := conn.PoolsConstraints().Bulk()
	bulk.UpdateAll(query, bson.M{"$push": bson.M{"values": newName}})
	bulk.UpdateAll(query, bson.M{"$pull": bson.M{"values": oldName}})
	_, err = bulk.Run()
	return err
}

func changeDefaultPool(ctx context.Context, force bool) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	p, err := listPools(ctx, bson.M{"default": true})
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
	teamConstraint, err := getExactConstraintForPool(poolName, ConstraintTypeTeam)
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
	return appendPoolConstraint(poolName, ConstraintTypeTeam, teams...)
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
	constraint, err := getExactConstraintForPool(poolName, ConstraintTypeTeam)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	if constraint != nil && constraint.Blacklist {
		return errors.New("Unable to remove teams from blacklist constraint")
	}
	return removePoolConstraint(poolName, ConstraintTypeTeam, teams...)
}

func ListPools(ctx context.Context, names ...string) ([]Pool, error) {
	return listPools(ctx, bson.M{"_id": bson.M{"$in": names}})
}

func ListAllPools(ctx context.Context) ([]Pool, error) {
	return listPools(ctx, nil)
}

func ListPublicPools(ctx context.Context) ([]Pool, error) {
	return getPoolsSatisfyConstraints(ctx, true, ConstraintTypeTeam, "*")
}

func ListPossiblePools(ctx context.Context, teams []string) ([]Pool, error) {
	return getPoolsSatisfyConstraints(ctx, false, ConstraintTypeTeam, teams...)
}

func ListPoolsForTeam(ctx context.Context, team string) ([]Pool, error) {
	return getPoolsSatisfyConstraints(ctx, true, ConstraintTypeTeam, team)
}

func listPools(ctx context.Context, query bson.M) ([]Pool, error) {
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

func GetProvisionerForPool(ctx context.Context, name string) (provision.Provisioner, error) {
	if name == "" {
		return provision.GetDefault()
	}
	prov := poolCache.Get(name)
	if prov != nil {
		return prov, nil
	}
	p, err := GetPoolByName(ctx, name)
	if err != nil {
		return nil, err
	}
	prov, err = p.GetProvisioner()
	if err != nil {
		return nil, err
	}
	poolCache.Set(name, prov)
	return prov, nil
}

// GetPoolByName finds a pool by name
func GetPoolByName(ctx context.Context, name string) (*Pool, error) {
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
	p.ctx = ctx
	return &p, nil
}

func GetDefaultPool(ctx context.Context) (*Pool, error) {
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
	pool.ctx = ctx
	return &pool, nil
}

func PoolUpdate(ctx context.Context, name string, opts UpdatePoolOptions) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = GetPoolByName(ctx, name)
	if err != nil {
		return err
	}
	if opts.Default != nil && *opts.Default {
		err = changeDefaultPool(ctx, opts.Force)
		if err != nil {
			return err
		}
	}
	query := bson.M{}
	if opts.Default != nil {
		query["default"] = *opts.Default
	}
	if (opts.Public != nil && *opts.Public) || (opts.Default != nil && *opts.Default) {
		errConstraint := SetPoolConstraint(&PoolConstraint{PoolExpr: name, Field: ConstraintTypeTeam, Values: []string{"*"}})
		if errConstraint != nil {
			return err
		}
	}
	if (opts.Public != nil && !*opts.Public) || (opts.Default != nil && !*opts.Default) {
		errConstraint := removePoolConstraint(name, ConstraintTypeTeam, "*")
		if errConstraint != nil {
			return err
		}
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

type poolService struct {
	storage provisionTypes.PoolStorage
}

var _ provisionTypes.PoolService = &poolService{}

func PoolStorage() (provisionTypes.PoolStorage, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return dbDriver.PoolStorage, nil
}

func PoolService() (provisionTypes.PoolService, error) {
	poolStorage, err := PoolStorage()
	if err != nil {
		return nil, err
	}
	return &poolService{storage: poolStorage}, nil
}

func (s *poolService) FindByName(ctx context.Context, name string) (*provisionTypes.Pool, error) {
	return s.storage.FindByName(ctx, name)
}

func (s *poolService) List(ctx context.Context) ([]provisionTypes.Pool, error) {
	return s.storage.FindAll(ctx)
}
