// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package galeb

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/router"
	galebClient "github.com/tsuru/tsuru/router/galeb/client"
)

const routerType = "galeb"

type galebRouter struct {
	client     *galebClient.GalebClient
	domain     string
	prefix     string
	routerName string
}

func init() {
	router.Register(routerType, createRouter)
	hc.AddChecker("Router galeb", router.BuildHealthCheck(routerType))
}

func createRouter(routerName, configPrefix string) (router.Router, error) {
	apiUrl, err := config.GetString(configPrefix + ":api-url")
	if err != nil {
		return nil, err
	}
	username, _ := config.GetString(configPrefix + ":username")
	password, _ := config.GetString(configPrefix + ":password")
	token, _ := config.GetString(configPrefix + ":token")
	tokenHeader, _ := config.GetString(configPrefix + ":token-header")
	if token == "" && (username == "" || password == "") {
		return nil, fmt.Errorf("either token or username and password must be set for galeb router")
	}
	domain, err := config.GetString(configPrefix + ":domain")
	if err != nil {
		return nil, err
	}
	environment, _ := config.GetString(configPrefix + ":environment")
	project, _ := config.GetString(configPrefix + ":project")
	balancePolicy, _ := config.GetString(configPrefix + ":balance-policy")
	ruleType, _ := config.GetString(configPrefix + ":rule-type")
	debug, _ := config.GetBool(configPrefix + ":debug")
	waitTimeoutSec, err := config.GetInt(configPrefix + ":wait-timeout")
	if err != nil {
		waitTimeoutSec = 10 * 60
	}
	client := galebClient.GalebClient{
		ApiUrl:        apiUrl,
		Username:      username,
		Password:      password,
		Token:         token,
		TokenHeader:   tokenHeader,
		Environment:   environment,
		Project:       project,
		BalancePolicy: balancePolicy,
		RuleType:      ruleType,
		WaitTimeout:   time.Duration(waitTimeoutSec) * time.Second,
		Debug:         debug,
	}
	r := galebRouter{
		client:     &client,
		domain:     domain,
		prefix:     configPrefix,
		routerName: routerName,
	}
	return &r, nil
}

func (r *galebRouter) poolName(base string) string {
	return fmt.Sprintf("tsuru-backendpool-%s-%s", r.routerName, base)
}

func (r *galebRouter) ruleName(base string) string {
	return fmt.Sprintf("tsuru-rootrule-%s-%s", r.routerName, base)
}

func (r *galebRouter) virtualHostName(base string) string {
	return fmt.Sprintf("%s.%s", base, r.domain)
}

func (r *galebRouter) AddBackend(name string) error {
	backendPoolId, err := r.client.AddBackendPool(r.poolName(name))
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
	ruleId, err := r.client.AddRuleToID(r.ruleName(name), backendPoolId)
	if err != nil {
		return err
	}
	err = r.client.SetRuleVirtualHostIDs(ruleId, virtualHostId)
	if err != nil {
		return err
	}
	return router.Store(name, name, routerType)
}

func (r *galebRouter) AddRoute(name string, address *url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	address.Scheme = router.HttpScheme
	_, err = r.client.AddBackend(address, r.poolName(backendName))
	if err == galebClient.ErrItemAlreadyExists {
		return router.ErrRouteExists
	}
	return err
}

func (r *galebRouter) AddRoutes(name string, addresses []*url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	for _, a := range addresses {
		a.Scheme = router.HttpScheme
	}
	return r.client.AddBackends(addresses, r.poolName(backendName))
}

func (r *galebRouter) RemoveRoute(name string, address *url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	targets, err := r.client.FindTargetsByParent(r.poolName(backendName))
	if err != nil {
		return err
	}
	var id string
	for _, target := range targets {
		if strings.HasSuffix(target.Name, address.Host) {
			id = target.FullId()
		}
	}
	if id == "" {
		return router.ErrRouteNotFound
	}
	return r.client.RemoveBackendByID(id)
}

func (r *galebRouter) RemoveRoutes(name string, addresses []*url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	addressMap := map[string]struct{}{}
	for _, addr := range addresses {
		addressMap[addr.Host] = struct{}{}
	}
	targets, err := r.client.FindTargetsByParent(r.poolName(backendName))
	if err != nil {
		return err
	}
	var ids []string
	for _, target := range targets {
		parsedAddr, err := url.Parse(target.Name)
		if err != nil {
			return err
		}
		if _, ok := addressMap[parsedAddr.Host]; ok {
			ids = append(ids, target.FullId())
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return r.client.RemoveBackendsByIDs(ids)
}

func (r *galebRouter) CNames(name string) ([]*url.URL, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	rule := r.ruleName(backendName)
	virtualhosts, err := r.client.FindVirtualHostsByRule(rule)
	if err != nil {
		return nil, err
	}
	urls := []*url.URL{}
	address, err := r.Addr(name)
	if err != nil {
		return nil, err
	}
	for _, vhost := range virtualhosts {
		if vhost.Name != address {
			urls = append(urls, &url.URL{Host: vhost.Name})
		}
	}
	return urls, nil
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
	return r.client.SetRuleVirtualHost(r.ruleName(backendName), cname)
}

func (r *galebRouter) UnsetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	err = r.client.RemoveRuleVirtualHost(r.ruleName(backendName), cname)
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

func (r *galebRouter) Swap(backend1, backend2 string, cnameOnly bool) error {
	return router.Swap(r, backend1, backend2, cnameOnly)
}

func (r *galebRouter) Routes(name string) ([]*url.URL, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	targets, err := r.client.FindTargetsByParent(r.poolName(backendName))
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
	rule := r.ruleName(backendName)
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
	err = r.client.RemoveRule(r.ruleName(backendName))
	if err != nil {
		return err
	}
	targets, err := r.client.FindTargetsByParent(r.poolName(backendName))
	if err != nil {
		return err
	}
	for _, target := range targets {
		r.client.RemoveBackendByID(target.FullId())
	}
	err = r.client.RemoveBackendPool(r.poolName(backendName))
	if err != nil {
		return err
	}
	return router.Remove(backendName)
}

func (r *galebRouter) SetHealthcheck(name string, data router.HealthcheckData) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if data.Path == "" {
		data.Path = "/"
	}
	poolProperties := galebClient.BackendPoolProperties{
		HcPath: data.Path,
		HcBody: data.Body,
	}
	if data.Status != 0 {
		poolProperties.HcStatusCode = fmt.Sprintf("%d", data.Status)
	}
	return r.client.UpdatePoolProperties(r.poolName(backendName), poolProperties)
}
