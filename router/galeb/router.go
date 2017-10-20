// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package galeb

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router"
	galebClient "github.com/tsuru/tsuru/router/galeb/client"
)

const routerType = "galeb"

var clientCache struct {
	sync.Mutex
	cache map[string]*galebClient.GalebClient
}

func getClient(configPrefix string) (*galebClient.GalebClient, error) {
	clientCache.Lock()
	defer clientCache.Unlock()
	if clientCache.cache == nil {
		clientCache.cache = map[string]*galebClient.GalebClient{}
	}
	if clientCache.cache[configPrefix] != nil {
		return clientCache.cache[configPrefix], nil
	}
	apiURL, err := config.GetString(configPrefix + ":api-url")
	if err != nil {
		return nil, err
	}
	username, _ := config.GetString(configPrefix + ":username")
	password, _ := config.GetString(configPrefix + ":password")
	tokenHeader, _ := config.GetString(configPrefix + ":token-header")
	useToken, _ := config.GetBool(configPrefix + ":use-token")
	environment, _ := config.GetString(configPrefix + ":environment")
	project, _ := config.GetString(configPrefix + ":project")
	balancePolicy, _ := config.GetString(configPrefix + ":balance-policy")
	ruleType, _ := config.GetString(configPrefix + ":rule-type")
	debug, _ := config.GetBool(configPrefix + ":debug")
	waitTimeoutSec, err := config.GetInt(configPrefix + ":wait-timeout")
	if err != nil {
		waitTimeoutSec = 10 * 60
	}
	client := &galebClient.GalebClient{
		ApiURL:        apiURL,
		Username:      username,
		Password:      password,
		UseToken:      useToken,
		TokenHeader:   tokenHeader,
		Environment:   environment,
		Project:       project,
		BalancePolicy: balancePolicy,
		RuleType:      ruleType,
		WaitTimeout:   time.Duration(waitTimeoutSec) * time.Second,
		Debug:         debug,
	}
	clientCache.cache[configPrefix] = client
	return client, nil
}

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
	domain, err := config.GetString(configPrefix + ":domain")
	if err != nil {
		return nil, err
	}
	client, err := getClient(configPrefix)
	if err != nil {
		return nil, err
	}
	r := galebRouter{
		client:     client,
		domain:     domain,
		prefix:     configPrefix,
		routerName: routerName,
	}
	return &r, nil
}

func (r *galebRouter) GetName() string {
	return r.routerName
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

func (r *galebRouter) AddBackend(app router.App) (err error) {
	name := app.GetName()
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendPoolId, err := r.client.AddBackendPool(r.poolName(name))
	if _, ok := errors.Cause(err).(galebClient.ErrItemAlreadyExists); ok {
		return router.ErrBackendExists
	}
	if err != nil {
		return err
	}
	virtualHostId, err := r.client.AddVirtualHost(r.virtualHostName(name))
	if err != nil {
		cleanupErr := r.forceCleanupBackend(name)
		if cleanupErr != nil {
			log.Errorf("unable to cleanup router after failure %+v", cleanupErr)
		}
		return err
	}
	ruleId, err := r.client.AddRuleToID(r.ruleName(name), backendPoolId)
	if err != nil {
		cleanupErr := r.forceCleanupBackend(name)
		if cleanupErr != nil {
			log.Errorf("unable to cleanup router after failure %+v", cleanupErr)
		}
		return err
	}
	err = r.client.SetRuleVirtualHostIDs(ruleId, virtualHostId)
	if err != nil {
		cleanupErr := r.forceCleanupBackend(name)
		if cleanupErr != nil {
			log.Errorf("unable to cleanup router after failure %+v", cleanupErr)
		}
		return err
	}
	err = router.Store(name, name, routerType)
	if err != nil {
		cleanupErr := r.forceCleanupBackend(name)
		if cleanupErr != nil {
			log.Errorf("unable to cleanup router after failure %+v", cleanupErr)
		}
		return err
	}
	return nil
}

func (r *galebRouter) AddRoutes(name string, addresses []*url.URL) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	for _, a := range addresses {
		a.Scheme = router.HttpScheme
	}
	return r.client.AddBackends(addresses, r.poolName(backendName))
}

func (r *galebRouter) RemoveRoutes(name string, addresses []*url.URL) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
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

func (r *galebRouter) CNames(name string) (urls []*url.URL, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	rule := r.ruleName(backendName)
	virtualhosts, err := r.client.FindVirtualHostsByRule(rule)
	if err != nil {
		return nil, err
	}
	urls = []*url.URL{}
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

func (r *galebRouter) SetCName(cname, name string) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if !router.ValidCName(cname, r.domain) {
		return router.ErrCNameNotAllowed
	}
	_, err = r.client.AddVirtualHost(cname)
	if _, ok := errors.Cause(err).(galebClient.ErrItemAlreadyExists); ok {
		return router.ErrCNameExists
	}
	if err != nil {
		return err
	}
	return r.client.SetRuleVirtualHost(r.ruleName(backendName), cname)
}

func (r *galebRouter) UnsetCName(cname, name string) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	err = r.client.RemoveRuleVirtualHost(r.ruleName(backendName), cname)
	if _, ok := errors.Cause(err).(galebClient.ErrItemNotFound); ok {
		return router.ErrCNameNotFound
	}
	if err != nil {
		return err
	}
	return r.client.RemoveVirtualHost(cname)
}

func (r *galebRouter) Addr(name string) (addr string, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return "", err
	}
	return r.virtualHostName(backendName), nil
}

func (r *galebRouter) Swap(backend1, backend2 string, cnameOnly bool) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	return router.Swap(r, backend1, backend2, cnameOnly)
}

func (r *galebRouter) Routes(name string) (urls []*url.URL, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	targets, err := r.client.FindTargetsByParent(r.poolName(backendName))
	if err != nil {
		return nil, err
	}
	urls = make([]*url.URL, len(targets))
	for i, target := range targets {
		urls[i], err = url.Parse(target.Name)
		if err != nil {
			return nil, err
		}
	}
	return urls, nil
}

func (r *galebRouter) StartupMessage() (string, error) {
	return fmt.Sprintf("galeb router %q with API URL %q.", r.domain, r.client.ApiURL), nil
}

func (r *galebRouter) HealthCheck() (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	return r.client.Healthcheck()
}

func (r *galebRouter) RemoveBackend(name string) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
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
		if _, ok := err.(galebClient.ErrItemNotFound); ok {
			return router.ErrBackendNotFound
		}
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
	return r.client.RemoveBackendPool(r.poolName(backendName))
}

func (r *galebRouter) forceCleanupBackend(backendName string) error {
	rule := r.ruleName(backendName)
	multiErr := tsuruErrors.NewMultiError()
	virtualhosts, err := r.client.FindVirtualHostsByRule(rule)
	if err == nil {
		for _, virtualhost := range virtualhosts {
			err = r.client.RemoveRuleVirtualHost(rule, virtualhost.Name)
			if err != nil {
				multiErr.Add(err)
			}
			err = r.client.RemoveVirtualHostByID(virtualhost.FullId())
			if err != nil {
				multiErr.Add(err)
			}
		}
	} else {
		multiErr.Add(err)
	}
	err = r.client.RemoveVirtualHost(r.virtualHostName(backendName))
	if err != nil {
		multiErr.Add(err)
	}
	err = r.client.RemoveRule(rule)
	if err != nil {
		multiErr.Add(err)
	}
	pool := r.poolName(backendName)
	targets, err := r.client.FindTargetsByParent(pool)
	if err == nil {
		for _, target := range targets {
			err = r.client.RemoveBackendByID(target.FullId())
			if err != nil {
				multiErr.Add(err)
			}
		}
	} else {
		multiErr.Add(err)
	}
	err = r.client.RemoveBackendPool(pool)
	if err != nil {
		multiErr.Add(err)
	}
	return multiErr.ToError()
}

func (r *galebRouter) SetHealthcheck(name string, data router.HealthcheckData) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
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
