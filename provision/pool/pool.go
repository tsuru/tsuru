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

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db/storagev2"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
	provisionTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/validation"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	apiv1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
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
	ErrPoolHasNoVolumePlan            = errors.New("no volume-plan found for pool")
	ErrPoolHasNoCertIssuer            = errors.New("no cert-issuer found for pool")
)

const (
	affinityKey = "affinity"
)

type Pool struct {
	Name        string `bson:"_id"`
	Default     bool
	Provisioner string

	Labels map[string]string
}

type PoolInfo struct {
	Pool
	Public  bool                            `json:"public"`
	Teams   []string                        `json:"teams"`
	Allowed map[PoolConstraintType][]string `json:"allowed"`
}

type AddPoolOptions struct {
	Name        string
	Public      bool
	Default     bool
	Force       bool
	Provisioner string

	Labels map[string]string
}

type UpdatePoolOptions struct {
	Default *bool
	Public  *bool
	Force   bool

	Labels map[string]string
}

func (p *Pool) GetAffinity() (*apiv1.Affinity, error) {
	if affinity, ok := p.Labels[affinityKey]; ok {
		var k8sAffinity apiv1.Affinity
		if err := yaml.Unmarshal([]byte(affinity), &k8sAffinity); err != nil {
			return nil, err
		}
		return &k8sAffinity, nil
	}

	return nil, nil
}

func (p *Pool) GetProvisioner() (provision.Provisioner, error) {
	if p.Provisioner != "" {
		return provision.Get(p.Provisioner)
	}
	return provision.GetDefault()
}

func (p *Pool) GetTeams(ctx context.Context) ([]string, error) {
	allowedValues, err := p.allowedValues(ctx)
	if err != nil {
		return nil, err
	}
	if c := allowedValues[ConstraintTypeTeam]; len(c) > 0 {
		return c, nil
	}
	return nil, ErrPoolHasNoTeam
}

func (p *Pool) GetRouters(ctx context.Context) ([]string, error) {
	allowedValues, err := p.allowedValues(ctx)
	if err != nil {
		return nil, err
	}
	if c := allowedValues[ConstraintTypeRouter]; len(c) > 0 {
		return c, nil
	}
	return nil, ErrPoolHasNoRouter
}

func (p *Pool) GetCertIssuers(ctx context.Context) ([]string, error) {
	allowedValues, err := p.allowedValues(ctx)
	if err != nil {
		return nil, err
	}
	if c := allowedValues[ConstraintTypeCertIssuer]; len(c) > 0 {
		return c, nil
	}
	return nil, ErrPoolHasNoCertIssuer
}

func (p *Pool) GetVolumePlans(ctx context.Context) ([]string, error) {
	allowedValues, err := p.allowedValues(ctx)
	if err != nil {
		return nil, err
	}

	if c := allowedValues[ConstraintTypeVolumePlan]; len(c) > 0 {
		return c, nil
	}

	return nil, ErrPoolHasNoVolumePlan
}

func (p *Pool) GetPlans(ctx context.Context) ([]string, error) {
	allowedValues, err := p.allowedValues(ctx)
	if err != nil {
		return nil, err
	}
	if c := allowedValues[ConstraintTypePlan]; len(c) > 0 {
		return c, nil
	}
	return nil, ErrPoolHasNoPlan
}

func (p *Pool) GetDefaultPlan(ctx context.Context) (*appTypes.Plan, error) {
	constraints, err := getConstraintsForPool(ctx, p.Name, ConstraintTypePlan)
	if err != nil {
		return nil, err
	}
	defaultPlan, err := servicemanager.Plan.DefaultPlan(ctx)
	if err != nil {
		return nil, err
	}
	constraint := constraints[ConstraintTypePlan]
	if constraint == nil || len(constraint.Values) == 0 {
		return defaultPlan, nil
	}
	if constraint.Blacklist || strings.Contains(constraint.Values[0], "*") {
		var allowed map[PoolConstraintType][]string
		var plan *appTypes.Plan
		allowed, err = p.allowedValues(ctx)
		if err != nil {
			return nil, err
		}
		if len(allowed[ConstraintTypePlan]) > 0 {
			plan, err = servicemanager.Plan.FindByName(ctx, allowed[ConstraintTypePlan][0])
			if err != nil {
				return nil, err
			}
			return plan, nil
		}
		return defaultPlan, nil
	}
	plan, err := servicemanager.Plan.FindByName(ctx, constraint.Values[0])
	if err != nil {
		return defaultPlan, nil
	}
	return plan, nil
}

func (p *Pool) GetDefaultRouter(ctx context.Context) (string, error) {
	constraints, err := getConstraintsForPool(ctx, p.Name, ConstraintTypeRouter)
	if err != nil {
		return "", err
	}
	constraint := constraints[ConstraintTypeRouter]
	if constraint == nil || len(constraint.Values) == 0 {
		return router.Default(ctx)
	}
	if constraint.Blacklist || strings.Contains(constraint.Values[0], "*") {
		var allowed map[PoolConstraintType][]string
		allowed, err = p.allowedValues(ctx)
		if err != nil {
			return "", err
		}
		if len(allowed[ConstraintTypeRouter]) == 1 {
			return allowed[ConstraintTypeRouter][0], nil
		}
		return router.Default(ctx)
	}
	routers, err := routersNames(ctx)
	if err != nil {
		return "", err
	}
	for _, r := range routers {
		if constraint.Values[0] == r {
			return r, nil
		}
	}
	return router.Default(ctx)
}

func (p *Pool) ValidateRouters(ctx context.Context, routers []appTypes.AppRouter) error {
	if len(routers) == 0 {
		return nil
	}

	availableRouters, err := p.GetRouters(ctx)
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

func (p *Pool) allowedValues(ctx context.Context) (map[PoolConstraintType][]string, error) {
	teams, err := teamsNames(ctx)
	if err != nil {
		return nil, err
	}
	routers, err := routersNames(ctx)
	if err != nil {
		return nil, err
	}
	services, err := servicesNames(ctx)
	if err != nil {
		return nil, err
	}
	plans, err := plansNames(ctx)
	if err != nil {
		return nil, err
	}
	volumePlans, err := volumePlanNames(ctx)
	if err != nil {
		return nil, err
	}

	resolved := map[PoolConstraintType][]string{
		ConstraintTypeRouter:     routers,
		ConstraintTypeService:    services,
		ConstraintTypeTeam:       teams,
		ConstraintTypePlan:       plans,
		ConstraintTypeVolumePlan: volumePlans,
	}
	constraints, err := getConstraintsForPool(ctx, p.Name, ConstraintTypeTeam, ConstraintTypeRouter, ConstraintTypeService, ConstraintTypePlan, ConstraintTypeVolumePlan, ConstraintTypeCertIssuer)
	if err != nil {
		return nil, err
	}
	for k, v := range constraints {
		// for cert-issuers, we apply the constraint directly to the cluster provider. There is no service on Tsuru to list this constraint type
		if k == ConstraintTypeCertIssuer {
			resolved[k] = v.Values
			continue
		}
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

func volumePlanNames(ctx context.Context) ([]string, error) {
	volumePlans, err := servicemanager.Volume.ListPlans(ctx)
	if err != nil {
		return nil, err
	}

	var pNames []string
	for _, vPlanList := range volumePlans {
		for _, vPlan := range vPlanList {
			pNames = append(pNames, vPlan.Name)
		}
	}

	return pNames, nil
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

func (p *Pool) Info(ctx context.Context) (*PoolInfo, error) {

	teams, err := getExactConstraintForPool(ctx, p.Name, ConstraintTypeTeam)
	if err != nil {
		return nil, err
	}
	resolvedConstraints, err := p.allowedValues(ctx)
	if err != nil {
		return nil, err
	}

	return &PoolInfo{
		Pool:    *p,
		Public:  teams.AllowsAll(),
		Teams:   resolvedConstraints[ConstraintTypeTeam],
		Allowed: resolvedConstraints,
	}, nil
}

func validateLabels(labels map[string]string) error {
	if affinityStr, ok := labels[affinityKey]; ok {
		var affinity apiv1.Affinity
		if err := json.Unmarshal([]byte(affinityStr), &affinity); err != nil {
			return err
		}
	}

	return nil
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

	if len(p.Labels) > 0 {
		return validateLabels(p.Labels)
	}
	return nil
}

func AddPool(ctx context.Context, opts AddPoolOptions) error {
	pool := Pool{Name: opts.Name, Default: opts.Default, Provisioner: opts.Provisioner, Labels: opts.Labels}
	if err := pool.validate(); err != nil {
		return err
	}
	collection, err := storagev2.PoolCollection()
	if err != nil {
		return err
	}
	if opts.Default {
		err = changeDefaultPool(ctx, opts.Force)
		if err != nil {
			return err
		}
	}
	_, err = collection.InsertOne(ctx, pool)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrPoolAlreadyExists
		}
		return err
	}
	if opts.Public || opts.Default {
		return SetPoolConstraint(ctx, &PoolConstraint{PoolExpr: opts.Name, Field: ConstraintTypeTeam, Values: []string{"*"}})
	}
	return nil
}

func RenamePoolTeam(ctx context.Context, oldName, newName string) error {
	collection, err := storagev2.PoolConstraintsCollection()
	if err != nil {
		return err
	}
	query := mongoBSON.M{
		"field":  "team",
		"values": oldName,
	}

	_, err = collection.BulkWrite(ctx, []mongo.WriteModel{
		mongo.NewUpdateManyModel().SetFilter(query).SetUpdate(mongoBSON.M{"$addToSet": mongoBSON.M{"values": newName}}),
		mongo.NewUpdateManyModel().SetFilter(query).SetUpdate(mongoBSON.M{"$pull": mongoBSON.M{"values": oldName}}),
	})

	if err != nil {
		return err
	}

	return nil
}

func changeDefaultPool(ctx context.Context, force bool) error {
	collection, err := storagev2.PoolCollection()
	if err != nil {
		return err
	}
	p, err := listPools(ctx, mongoBSON.M{"default": true})
	if err != nil {
		return err
	}
	if len(p) > 0 {
		if !force {
			return ErrDefaultPoolAlreadyExists
		}
		_, err = collection.UpdateOne(ctx, mongoBSON.M{"_id": p[0].Name}, mongoBSON.M{"$set": mongoBSON.M{"default": false}})
		return err
	}
	return nil
}

func RemovePool(ctx context.Context, poolName string) error {
	collection, err := storagev2.PoolCollection()
	if err != nil {
		return err
	}
	result, err := collection.DeleteOne(ctx, mongoBSON.M{"_id": poolName})
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return ErrPoolNotFound
		}
		return err

	}

	if result.DeletedCount == 0 {
		return ErrPoolNotFound
	}

	return err
}

func AddTeamsToPool(ctx context.Context, poolName string, teams []string) error {
	collection, err := storagev2.PoolCollection()
	if err != nil {
		return err
	}
	var pool Pool
	err = collection.FindOne(ctx, mongoBSON.M{"_id": poolName}).Decode(&pool)
	if err == mongo.ErrNoDocuments {
		return ErrPoolNotFound
	}
	if err != nil {
		return err
	}
	teamConstraint, err := getExactConstraintForPool(ctx, poolName, ConstraintTypeTeam)
	if err != nil && err != mongo.ErrNoDocuments {
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
	return appendPoolConstraint(ctx, poolName, ConstraintTypeTeam, teams...)
}

func RemoveTeamsFromPool(ctx context.Context, poolName string, teams []string) error {
	collection, err := storagev2.PoolCollection()
	if err != nil {
		return err
	}
	var pool Pool
	err = collection.FindOne(ctx, mongoBSON.M{"_id": poolName}).Decode(&pool)
	if err == mongo.ErrNoDocuments {
		return ErrPoolNotFound
	}
	if err != nil {
		return err
	}
	constraint, err := getExactConstraintForPool(ctx, poolName, ConstraintTypeTeam)
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}
	if constraint != nil && constraint.Blacklist {
		return errors.New("Unable to remove teams from blacklist constraint")
	}
	return removePoolConstraint(ctx, poolName, ConstraintTypeTeam, teams...)
}

func ListPools(ctx context.Context, names ...string) ([]Pool, error) {
	return listPools(ctx, mongoBSON.M{"_id": mongoBSON.M{"$in": names}})
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

func listPools(ctx context.Context, query mongoBSON.M) ([]Pool, error) {
	collection, err := storagev2.PoolCollection()
	if err != nil {
		return nil, err
	}
	pools := []Pool{}
	cursor, err := collection.Find(ctx, query)
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &pools)
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
	collection, err := storagev2.PoolCollection()
	if err != nil {
		return nil, err
	}
	var p Pool
	err = collection.FindOne(ctx, mongoBSON.M{"_id": name}).Decode(&p)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrPoolNotFound
		}
		return nil, err
	}
	return &p, nil
}

func ValidatePoolService(ctx context.Context, pool string, services []string) error {
	poolServices, err := servicemanager.Pool.Services(ctx, pool)
	if err != nil {
		return err
	}
	for _, svc := range services {
		if !contains(poolServices, svc) {
			msg := fmt.Sprintf("service %q is not available for pool %q.", svc, pool)

			if len(poolServices) > 0 {
				msg += fmt.Sprintf(" Available services are: %q", strings.Join(poolServices, ", "))
			}
			return &tsuruErrors.ValidationError{Message: msg}
		}
	}
	return nil
}

func contains(arr []string, c string) bool {
	for _, item := range arr {
		if item == c {
			return true
		}
	}
	return false
}

func GetDefaultPool(ctx context.Context) (*Pool, error) {
	collection, err := storagev2.PoolCollection()
	if err != nil {
		return nil, err
	}
	var pool Pool
	err = collection.FindOne(ctx, mongoBSON.M{"default": true}).Decode(&pool)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrPoolNotFound
		}
		return nil, err
	}
	return &pool, nil
}

func PoolUpdate(ctx context.Context, name string, opts UpdatePoolOptions) error {
	_, err := GetPoolByName(ctx, name)
	if err != nil {
		return err
	}
	if opts.Default != nil && *opts.Default {
		err = changeDefaultPool(ctx, opts.Force)
		if err != nil {
			return err
		}
	}
	query := mongoBSON.M{}
	if opts.Default != nil {
		query["default"] = *opts.Default
	}
	if len(opts.Labels) > 0 {
		if err = validateLabels(opts.Labels); err != nil {
			return err
		}
	}
	if opts.Labels != nil {
		query["labels"] = opts.Labels
	}
	if (opts.Public != nil && *opts.Public) || (opts.Default != nil && *opts.Default) {
		errConstraint := SetPoolConstraint(ctx, &PoolConstraint{PoolExpr: name, Field: ConstraintTypeTeam, Values: []string{"*"}})
		if errConstraint != nil {
			return err
		}
	}
	if (opts.Public != nil && !*opts.Public) || (opts.Default != nil && !*opts.Default) {
		errConstraint := removePoolConstraint(ctx, name, ConstraintTypeTeam, "*")
		if errConstraint != nil {
			return err
		}
	}
	if len(query) == 0 {
		return nil
	}

	collection, err := storagev2.PoolCollection()
	if err != nil {
		return err
	}
	result, err := collection.UpdateOne(ctx, mongoBSON.M{"_id": name}, mongoBSON.M{"$set": query})
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return ErrPoolNotFound
		}
		return err
	}

	if result.MatchedCount == 0 {
		return ErrPoolNotFound
	}

	return nil
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

func (s *poolService) Services(ctx context.Context, pool string) ([]string, error) {
	allowedValues, err := s.allowedValues(ctx, pool)
	if err != nil {
		return nil, err
	}
	if c := allowedValues[ConstraintTypeService]; len(c) > 0 {
		return c, nil
	}
	return nil, ErrPoolHasNoService
}

func (p *poolService) allowedValues(ctx context.Context, pool string) (map[PoolConstraintType][]string, error) {
	teams, err := teamsNames(ctx)
	if err != nil {
		return nil, err
	}
	routers, err := routersNames(ctx)
	if err != nil {
		return nil, err
	}
	services, err := servicesNames(ctx)
	if err != nil {
		return nil, err
	}
	plans, err := plansNames(ctx)
	if err != nil {
		return nil, err
	}
	volumePlans, err := volumePlanNames(ctx)
	if err != nil {
		return nil, err
	}

	resolved := map[PoolConstraintType][]string{
		ConstraintTypeRouter:     routers,
		ConstraintTypeService:    services,
		ConstraintTypeTeam:       teams,
		ConstraintTypePlan:       plans,
		ConstraintTypeVolumePlan: volumePlans,
	}
	constraints, err := getConstraintsForPool(ctx, pool, ConstraintTypeTeam, ConstraintTypeRouter, ConstraintTypeService, ConstraintTypePlan, ConstraintTypeVolumePlan)
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
