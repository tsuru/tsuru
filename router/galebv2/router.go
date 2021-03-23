// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package galebv2

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router"
	galebClient "github.com/tsuru/tsuru/router/galebv2/client"
	appTypes "github.com/tsuru/tsuru/types/app"
	routerTypes "github.com/tsuru/tsuru/types/router"
)

const routerType = "galebv2"

var (
	_ router.Router                  = &galebRouter{}
	_ router.AsyncRouter             = &galebRouter{}
	_ router.CustomHealthcheckRouter = &galebRouter{}
	_ router.HealthChecker           = &galebRouter{}
	_ router.MessageRouter           = &galebRouter{}
	_ router.CNameMoveRouter         = &galebRouter{}
	_ router.CNameRouter             = &galebRouter{}
	_ router.PrefixRouter            = &galebRouter{}
)

var clientCache struct {
	sync.Mutex
	cache map[string]*galebClient.GalebClient
}

func getClient(routerName string, config router.ConfigGetter) (*galebClient.GalebClient, error) {
	clientCache.Lock()
	defer clientCache.Unlock()
	if clientCache.cache == nil {
		clientCache.cache = map[string]*galebClient.GalebClient{}
	}
	key, err := config.Hash()
	if err != nil {
		return nil, err
	}
	if clientCache.cache[key] != nil {
		return clientCache.cache[key], nil
	}
	apiURL, err := config.GetString("api-url")
	if err != nil {
		return nil, err
	}
	username, _ := config.GetString("username")
	password, _ := config.GetString("password")
	tokenHeader, _ := config.GetString("token-header")
	useToken, _ := config.GetBool("use-token")
	environment, _ := config.GetString("environment")
	project, _ := config.GetString("project")
	balancePolicy, _ := config.GetString("balance-policy")
	ruleType, _ := config.GetString("rule-type")
	debug, _ := config.GetBool("debug")
	waitTimeoutSec, err := config.GetInt("wait-timeout")
	if err != nil {
		waitTimeoutSec = 10 * 60
	}
	maxRequests, _ := config.GetInt("max-requests")
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
		MaxRequests:   maxRequests,
	}
	clientCache.cache[key] = client
	return client, nil
}

type galebRouter struct {
	client     *galebClient.GalebClient
	domain     string
	config     router.ConfigGetter
	routerName string
}

func init() {
	router.Register(routerType, createRouter)
	hc.AddChecker("Router galeb", router.BuildHealthCheck(routerType))
}

func createRouter(routerName string, config router.ConfigGetter) (router.Router, error) {
	domain, err := config.GetString("domain")
	if err != nil {
		return nil, err
	}
	client, err := getClient(routerName, config)
	if err != nil {
		return nil, err
	}
	r := galebRouter{
		client:     client,
		domain:     domain,
		config:     config,
		routerName: routerName,
	}
	return &r, nil
}

func (r *galebRouter) GetName() string {
	return r.routerName
}

func (r *galebRouter) GetType() string {
	return routerType
}

func (r *galebRouter) poolName(prefix, base string) string {
	if prefix != "" {
		prefix = galebClient.RoutePrefixSeparator + prefix
	}
	return fmt.Sprintf("tsuru-backendpool-%s-%s%s", r.routerName, base, prefix)
}

func (r *galebRouter) ruleName(prefix, base string) string {
	if prefix != "" {
		prefix = galebClient.RoutePrefixSeparator + prefix
	}
	return fmt.Sprintf("tsuru-rootrule-%s-%s%s", r.routerName, base, prefix)
}

func (r *galebRouter) virtualHostName(prefix, base string) string {
	if prefix != "" {
		prefix = prefix + "."
	}
	return fmt.Sprintf("%s%s.%s", prefix, base, r.domain)
}

func (r *galebRouter) poolNameToPrefix(poolName, base string) string {
	return strings.TrimPrefix(strings.TrimPrefix(poolName, r.poolName("", base)), galebClient.RoutePrefixSeparator)
}

func (r *galebRouter) AddBackend(ctx context.Context, app router.App) (err error) {
	return r.addBackend(ctx, app.GetName(), "", true)
}

func (r *galebRouter) AddBackendAsync(ctx context.Context, app router.App) (err error) {
	return r.addBackend(ctx, app.GetName(), "", false)
}

func (r *galebRouter) addBackend(ctx context.Context, name, prefix string, wait bool) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	poolName := r.poolName(prefix, name)
	ruleName := r.ruleName(prefix, name)
	vhName := r.virtualHostName(prefix, name)
	backendExists := false
	anyResourceCreated := false
	defer func() {
		if err != nil && anyResourceCreated && !backendExists {
			cleanupErr := r.forceCleanupBackend(ctx, name, prefix)
			if cleanupErr != nil {
				log.Errorf("unable to cleanup router after failure %+v", cleanupErr)
			}
		}
	}()
	_, err = r.client.AddBackendPool(ctx, poolName, wait)
	if galebClient.IsErrExists(err) {
		backendExists = true
	} else if err != nil {
		return err
	}
	anyResourceCreated = true
	_, err = r.client.AddVirtualHost(ctx, vhName, wait)
	if galebClient.IsErrExists(err) {
		backendExists = true
	} else if err != nil {
		return err
	}
	_, err = r.client.AddRuleToPool(ctx, ruleName, poolName)
	if galebClient.IsErrExists(err) {
		backendExists = true
	} else if err != nil {
		return err
	}
	err = r.client.SetRuleVirtualHost(ctx, ruleName, vhName, wait)
	if galebClient.IsErrExists(err) {
		backendExists = true
	} else if err != nil {
		return err
	}
	if backendExists {
		return router.ErrBackendExists
	}
	err = router.Store(name, name, routerType)
	if err != nil {
		return err
	}
	return nil
}

func (r *galebRouter) AddRoutes(ctx context.Context, app router.App, addresses []*url.URL) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	return r.addRoutes(ctx, app, "", addresses, true)
}

func (r *galebRouter) AddRoutesAsync(ctx context.Context, app router.App, addresses []*url.URL) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	return r.addRoutes(ctx, app, "", addresses, false)
}

func (r *galebRouter) addRoutes(ctx context.Context, app router.App, prefix string, addresses []*url.URL, wait bool) error {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	for _, a := range addresses {
		a.Scheme = router.HttpScheme
	}
	return r.client.AddBackends(ctx, addresses, r.poolName(prefix, backendName), wait)
}

func (r *galebRouter) RemoveRoutes(ctx context.Context, app router.App, addresses []*url.URL) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	return r.removeRoutes(ctx, app, "", addresses, true)
}

func (r *galebRouter) RemoveRoutesAsync(ctx context.Context, app router.App, addresses []*url.URL) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	return r.removeRoutes(ctx, app, "", addresses, false)
}

func (r *galebRouter) removeRoutes(ctx context.Context, app router.App, prefix string, addresses []*url.URL, wait bool) error {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	addressMap := map[string]struct{}{}
	for _, addr := range addresses {
		addressMap[addr.Host] = struct{}{}
	}
	targets, err := r.client.FindTargetsByPool(ctx, r.poolName(prefix, backendName))
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
	return r.client.RemoveResourcesByIDs(ctx, ids, wait)
}

func (r *galebRouter) CNames(ctx context.Context, app router.App) (urls []*url.URL, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return nil, err
	}
	virtualhost := r.virtualHostName("", backendName)
	virtualhosts, err := r.client.FindVirtualHostsByGroup(ctx, virtualhost)
	if err != nil {
		return nil, err
	}
	urls = []*url.URL{}
	address, err := r.Addr(ctx, app)
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

func (r *galebRouter) SetCName(ctx context.Context, cname string, app router.App) (err error) {
	return r.setCName(ctx, cname, app, true)
}

func (r *galebRouter) SetCNameAsync(ctx context.Context, cname string, app router.App) (err error) {
	return r.setCName(ctx, cname, app, false)
}

func (r *galebRouter) setCName(ctx context.Context, cname string, app router.App, wait bool) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	if !router.ValidCName(cname, r.domain) {
		return router.ErrCNameNotAllowed
	}
	virtualhost := r.virtualHostName("", backendName)
	_, err = r.client.AddVirtualHostWithGroup(ctx, cname, virtualhost, wait)
	if !galebClient.IsErrExists(err) {
		return err
	}
	err = r.client.UpdateVirtualHostWithGroup(ctx, cname, virtualhost, wait)
	if err != nil {
		return err
	}
	return router.ErrCNameExists
}

func (r *galebRouter) UnsetCName(ctx context.Context, cname string, app router.App) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	err = r.client.RemoveVirtualHost(ctx, cname)
	if _, ok := errors.Cause(err).(galebClient.ErrItemNotFound); ok {
		return router.ErrCNameNotFound
	}
	return err
}

func (r *galebRouter) MoveCName(ctx context.Context, cname string, orgApp, dstApp router.App) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	dstBackendName, err := router.Retrieve(dstApp.GetName())
	if err != nil {
		return err
	}
	_, err = r.client.AddVirtualHostWithGroup(ctx, cname, r.virtualHostName("", dstBackendName), true)
	if err != nil && !galebClient.IsErrExists(err) {
		return err
	}
	err = r.client.UpdateVirtualHostWithGroup(ctx, cname, r.virtualHostName("", dstBackendName), true)
	return err
}

func (r *galebRouter) Addr(ctx context.Context, app router.App) (addr string, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return "", err
	}
	return r.virtualHostName("", backendName), nil
}

func (r *galebRouter) Swap(ctx context.Context, app1, app2 router.App, cnameOnly bool) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	return router.Swap(ctx, r, app1, app2, cnameOnly)
}

func (r *galebRouter) Routes(ctx context.Context, app router.App) (urls []*url.URL, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return nil, err
	}
	targets, err := r.client.FindTargetsByPool(ctx, r.poolName("", backendName))
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

func (r *galebRouter) HealthCheck(ctx context.Context) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	return r.client.Healthcheck(ctx)
}

func (r *galebRouter) RemoveBackend(ctx context.Context, app router.App) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	if backendName != app.GetName() {
		return router.ErrBackendSwapped
	}
	poolTargets, err := r.client.FindAllTargetsByPoolPrefix(ctx, r.poolName("", backendName))
	if err != nil {
		return err
	}
	for poolName := range poolTargets {
		prefix := r.poolNameToPrefix(poolName, backendName)
		if prefix == "" {
			continue
		}
		err = r.removeBackendPrefix(ctx, backendName, prefix)
		if err != nil {
			return err
		}
	}
	return r.removeBackendPrefix(ctx, backendName, "")
}

func (r *galebRouter) removeBackendPrefix(ctx context.Context, backendName, prefix string) (err error) {
	virtualhost := r.virtualHostName(prefix, backendName)
	virtualhosts, err := r.client.FindVirtualHostsByGroup(ctx, virtualhost)
	if err != nil {
		if _, ok := errors.Cause(err).(galebClient.ErrItemNotFound); ok {
			return router.ErrBackendNotFound
		}
		return err
	}
	targets, err := r.client.FindTargetsByPool(ctx, r.poolName(prefix, backendName))
	if err != nil {
		return err
	}
	err = r.client.RemoveRulesOrderedByRule(ctx, r.ruleName(prefix, backendName))
	if err != nil {
		return err
	}
	err = r.client.RemoveRule(ctx, r.ruleName(prefix, backendName))
	if err != nil {
		return err
	}
	for _, target := range targets {
		err = r.client.RemoveResourceByID(ctx, target.FullId())
		if err != nil {
			return err
		}
	}
	err = r.client.RemoveBackendPool(ctx, r.poolName(prefix, backendName))
	if err != nil {
		return err
	}
	for _, vh := range virtualhosts {
		err = r.client.RemoveResourceByID(ctx, vh.FullId())
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *galebRouter) forceCleanupBackend(ctx context.Context, backendName, prefix string) error {
	rule := r.ruleName(prefix, backendName)
	virtualhostName := r.virtualHostName(prefix, backendName)
	multiErr := tsuruErrors.NewMultiError()
	virtualhosts, err := r.client.FindVirtualHostsByGroup(ctx, virtualhostName)
	if err == nil {
		for _, virtualhost := range virtualhosts {
			err = r.client.RemoveResourceByID(ctx, virtualhost.FullId())
			if err != nil {
				multiErr.Add(err)
			}
		}
	} else {
		multiErr.Add(err)
	}
	err = r.client.RemoveVirtualHost(ctx, r.virtualHostName(prefix, backendName))
	if err != nil {
		multiErr.Add(err)
	}
	err = r.client.RemoveRule(ctx, rule)
	if err != nil {
		multiErr.Add(err)
	}
	pool := r.poolName(prefix, backendName)
	targets, err := r.client.FindTargetsByPool(ctx, pool)
	if err == nil {
		for _, target := range targets {
			err = r.client.RemoveResourceByID(ctx, target.FullId())
			if err != nil {
				multiErr.Add(err)
			}
		}
	} else {
		multiErr.Add(err)
	}
	err = r.client.RemoveBackendPool(ctx, pool)
	if err != nil {
		multiErr.Add(err)
	}
	return multiErr.ToError()
}

func (r *galebRouter) SetHealthcheck(ctx context.Context, app router.App, data routerTypes.HealthcheckData) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	poolName := r.poolName("", backendName)
	if data.TCPOnly {
		return r.client.UpdatePoolProperties(ctx, poolName, galebClient.BackendPoolHealthCheck{
			HcTCPOnly: true,
		})
	}
	if data.Path == "" {
		data.Path = "/"
	}
	poolHealthCheck := galebClient.BackendPoolHealthCheck{
		HcPath: data.Path,
		HcBody: data.Body,
	}
	if data.Status != 0 {
		poolHealthCheck.HcHTTPStatusCode = fmt.Sprintf("%d", data.Status)
	}
	return r.client.UpdatePoolProperties(ctx, poolName, poolHealthCheck)
}

func (r *galebRouter) RoutesPrefix(ctx context.Context, app router.App) (addrs []appTypes.RoutableAddresses, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return nil, err
	}
	poolTargets, err := r.client.FindAllTargetsByPoolPrefix(ctx, r.poolName("", backendName))
	if err != nil {
		return nil, err
	}
	for poolName, targets := range poolTargets {
		prefix := r.poolNameToPrefix(poolName, backendName)
		urls := make([]*url.URL, len(targets))
		for i, target := range targets {
			urls[i], err = url.Parse(target.Name)
			if err != nil {
				return nil, err
			}
		}
		addrs = append(addrs, appTypes.RoutableAddresses{
			Prefix:    prefix,
			Addresses: urls,
		})
	}
	return addrs, nil
}

func (r *galebRouter) Addresses(ctx context.Context, app router.App) (addrs []string, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return nil, err
	}
	poolTargets, err := r.client.FindAllTargetsByPoolPrefix(ctx, r.poolName("", backendName))
	if err != nil {
		return nil, err
	}
	for poolName := range poolTargets {
		prefix := r.poolNameToPrefix(poolName, backendName)
		addrs = append(addrs, r.virtualHostName(prefix, app.GetName()))
	}
	return addrs, nil
}

func (r *galebRouter) AddRoutesPrefix(ctx context.Context, app router.App, addresses appTypes.RoutableAddresses, sync bool) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	if addresses.Prefix != "" {
		err = r.addBackend(ctx, backendName, addresses.Prefix, true)
		if err != nil && err != router.ErrBackendExists {
			return err
		}
	}
	return r.addRoutes(ctx, app, addresses.Prefix, addresses.Addresses, sync)
}

func (r *galebRouter) RemoveRoutesPrefix(ctx context.Context, app router.App, addresses appTypes.RoutableAddresses, sync bool) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	err = r.removeRoutes(ctx, app, addresses.Prefix, addresses.Addresses, false)
	if err != nil {
		return err
	}
	if addresses.Prefix == "" {
		return nil
	}
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	targets, err := r.client.FindTargetsByPool(ctx, r.poolName(addresses.Prefix, backendName))
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		r.removeBackendPrefix(ctx, backendName, addresses.Prefix)
	}
	return nil
}
