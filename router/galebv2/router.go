// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package galebv2

import (
	"fmt"
	"net/url"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/router"
	galebClient "github.com/tsuru/tsuru/router/galebv2/client"
)

const routerName = "galebv2"

type galebRouter struct {
	client *galebClient.GalebClient
	domain string
	prefix string
}

func init() {
	router.Register(routerName, createRouter)
	hc.AddChecker("Router galebv2", router.BuildHealthCheck(routerName))
}

func createRouter(prefix string) (router.Router, error) {
	apiUrl, err := config.GetString(prefix + ":api-url")
	if err != nil {
		return nil, err
	}
	username, err := config.GetString(prefix + ":username")
	if err != nil {
		return nil, err
	}
	password, err := config.GetString(prefix + ":password")
	if err != nil {
		return nil, err
	}
	domain, err := config.GetString(prefix + ":domain")
	if err != nil {
		return nil, err
	}
	environment, _ := config.GetString(prefix + ":environment")
	project, _ := config.GetString(prefix + ":project")
	balancePolicy, _ := config.GetString(prefix + ":balance-policy")
	ruleType, _ := config.GetString(prefix + ":rule-type")
	targetTypeBackend, _ := config.GetString(prefix + ":target-type-backend")
	targetTypePool, _ := config.GetString(prefix + ":target-type-backend-pool")
	client := galebClient.GalebClient{
		ApiUrl:            apiUrl,
		Username:          username,
		Password:          password,
		Environment:       environment,
		Project:           project,
		BalancePolicy:     balancePolicy,
		RuleType:          ruleType,
		TargetTypeBackend: targetTypeBackend,
		TargetTypePool:    targetTypePool,
	}
	r := galebRouter{
		client: &client,
		domain: domain,
		prefix: prefix,
	}
	return &r, nil
}

func poolName(base string) string {
	return fmt.Sprintf("tsuru-backendpool-%s", base)
}

func ruleName(base string) string {
	return fmt.Sprintf("tsuru-rootrule-%s", base)
}

func (r *galebRouter) virtualHostName(base string) string {
	return fmt.Sprintf("%s.%s", base, r.domain)
}

func (r *galebRouter) AddBackend(name string) error {
	backendPoolId, err := r.client.AddBackendPool(poolName(name))
	if err == galebClient.ErrItemAlreadyExists {
		return router.ErrBackendExists
	}
	if err != nil {
		return err
	}
	virtualHostId, err := r.client.AddVirtualHost(r.virtualHostName(name))
	if err != nil {
		return err
	}
	ruleId, err := r.client.AddRuleToID(ruleName(name), backendPoolId)
	if err != nil {
		return err
	}
	err = r.client.SetRuleVirtualHostIDs(ruleId, virtualHostId)
	if err != nil {
		return err
	}
	return router.Store(name, name, routerName)
}

func (r *galebRouter) AddRoute(name string, address *url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	_, err = r.client.AddBackend(address, poolName(backendName))
	if err == galebClient.ErrItemAlreadyExists {
		return router.ErrRouteExists
	}
	return err
}

func (r *galebRouter) RemoveRoute(name string, address *url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	targets, err := r.client.FindTargetsByParent(poolName(backendName))
	if err != nil {
		return err
	}
	var id string
	for _, target := range targets {
		if target.Name == address.String() {
			id = target.FullId()
		}
	}
	if id == "" {
		return router.ErrRouteNotFound
	}
	return r.client.RemoveBackendByID(id)
}

func (r *galebRouter) SetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if !router.ValidCName(cname, r.domain) {
		return router.ErrCNameNotAllowed
	}
	_, err = r.client.AddVirtualHost(cname)
	if err == galebClient.ErrItemAlreadyExists {
		return router.ErrCNameExists
	}
	if err != nil {
		return err
	}
	return r.client.SetRuleVirtualHost(ruleName(backendName), cname)
}

func (r *galebRouter) UnsetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	err = r.client.RemoveRuleVirtualHost(ruleName(backendName), cname)
	if err == galebClient.ErrItemNotFound {
		return router.ErrCNameNotFound
	}
	if err != nil {
		return err
	}
	return r.client.RemoveVirtualHost(cname)
}

func (r *galebRouter) Addr(name string) (string, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return "", err
	}
	return r.virtualHostName(backendName), nil
}

func (r *galebRouter) Swap(backend1, backend2 string) error {
	return router.Swap(r, backend1, backend2)
}

func (r *galebRouter) Routes(name string) ([]*url.URL, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	targets, err := r.client.FindTargetsByParent(poolName(backendName))
	if err != nil {
		return nil, err
	}
	urls := make([]*url.URL, len(targets))
	for i, target := range targets {
		urls[i], err = url.Parse(target.Name)
		if err != nil {
			return nil, err
		}
	}
	return urls, nil
}

func (r *galebRouter) StartupMessage() (string, error) {
	return fmt.Sprintf("galeb router %q with API URL %q.", r.domain, r.client.ApiUrl), nil
}

func (r *galebRouter) HealthCheck() error {
	return r.client.Healthcheck()
}

func (r *galebRouter) RemoveBackend(name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if backendName != name {
		return router.ErrBackendSwapped
	}
	rule := ruleName(backendName)
	virtualhosts, err := r.client.FindVirtualHostsByRule(rule)
	if err != nil {
		return err
	}
	for _, virtualhost := range virtualhosts {
		err = r.client.RemoveRuleVirtualHost(rule, virtualhost.Name)
		if err != nil {
			return err
		}
		err = r.client.RemoveVirtualHostByID(virtualhost.FullId())
		if err != nil {
			return err
		}
	}
	err = r.client.RemoveRule(ruleName(backendName))
	if err != nil {
		return err
	}
	targets, err := r.client.FindTargetsByParent(poolName(backendName))
	if err != nil {
		return err
	}
	for _, target := range targets {
		r.client.RemoveBackendByID(target.FullId())
	}
	err = r.client.RemoveBackendPool(poolName(backendName))
	if err != nil {
		return err
	}
	return router.Remove(backendName)
}
