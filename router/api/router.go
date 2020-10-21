// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	poolMultiCluster "github.com/tsuru/tsuru/provision/pool/multicluster"
	"github.com/tsuru/tsuru/router"
	appTypes "github.com/tsuru/tsuru/types/app"
	routerTypes "github.com/tsuru/tsuru/types/router"
)

//go:generate bash -c "rm -f routeriface.go && go run ./generator/combinations.go -o routeriface.go"

const routerType = "api"

var (
	_ router.OptsRouter              = &apiRouter{}
	_ router.Router                  = &apiRouter{}
	_ router.MessageRouter           = &apiRouter{}
	_ router.HealthChecker           = &apiRouter{}
	_ router.TLSRouter               = &apiRouterWithTLSSupport{}
	_ router.CNameRouter             = &apiRouterWithCnameSupport{}
	_ router.CustomHealthcheckRouter = &apiRouterWithHealthcheckSupport{}
	_ router.InfoRouter              = &apiRouterWithInfo{}
	_ router.StatusRouter            = &apiRouterWithStatus{}
	_ router.PrefixRouter            = &apiRouterWithPrefix{}
)

type apiRouter struct {
	routerName string
	endpoint   string
	headers    http.Header
	client     *http.Client
	supIface   router.Router

	debug        bool
	multiCluster bool
}

type apiRouterWithCnameSupport struct{ *apiRouter }

type apiRouterWithTLSSupport struct{ *apiRouter }

type apiRouterWithHealthcheckSupport struct{ *apiRouter }

type apiRouterWithInfo struct{ *apiRouter }

type apiRouterWithStatus struct{ *apiRouter }

type apiRouterWithPrefix struct{ *apiRouter }

type routesReq struct {
	Prefix    string            `json:"prefix"`
	Addresses []string          `json:"addresses"`
	ExtraData map[string]string `json:"extraData"`
}

type routesPrefixReq struct {
	Addresses           []string    `json:"addresses"`
	AddressesWithPrefix []routesReq `json:"addressesWithPrefix"`
}

type swapReq struct {
	Target    string `json:"target"`
	CnameOnly bool   `json:"cnameOnly"`
}

type cnamesResp struct {
	Cnames []string `json:"cnames"`
}

type certData struct {
	Certificate string `json:"certificate"`
	Key         string `json:"key"`
}

type backendResp struct {
	Address   string   `json:"address"`
	Addresses []string `json:"addresses"`
}

type statusResp struct {
	Status router.BackendStatus `json:"status"`
	Detail string               `json:"detail"`
}

type capability string

var (
	capCName       = capability("cname")
	capTLS         = capability("tls")
	capHealthcheck = capability("healthcheck")
	capInfo        = capability("info")
	capStatus      = capability("status")
	capPrefix      = capability("prefix")

	allCaps = []capability{capCName, capTLS, capHealthcheck, capInfo, capStatus, capPrefix}
)

func init() {
	router.Register(routerType, createRouter)
}

func createRouter(routerName string, config router.ConfigGetter) (router.Router, error) {
	endpoint, err := config.GetString("api-url")
	if err != nil {
		return nil, err
	}
	debug, _ := config.GetBool("debug")
	multiCluster, _ := config.GetBool("multi-cluster")
	headers, err := headersFromConfig(config)
	if err != nil {
		return nil, err
	}
	baseRouter := &apiRouter{
		routerName: routerName,
		endpoint:   endpoint,
		client:     net.Dial15Full60ClientNoKeepAlive,
		debug:      debug,
		headers:    headers,

		multiCluster: multiCluster,
	}
	baseRouter.supIface = toSupportedInterface(baseRouter, baseRouter.checkAllCapabilities(context.Background()))
	return baseRouter.supIface, nil
}

func (r *apiRouter) GetName() string {
	return r.routerName
}

func (r *apiRouter) AddBackend(ctx context.Context, app router.App) (err error) {
	return r.AddBackendOpts(ctx, app, nil)
}

func (r *apiRouter) AddBackendOpts(ctx context.Context, app router.App, opts map[string]string) error {
	err := r.doBackendOpts(ctx, app, http.MethodPost, opts)
	if err != nil {
		return err
	}
	return router.Store(app.GetName(), app.GetName(), routerType)
}

func (r *apiRouter) UpdateBackendOpts(ctx context.Context, app router.App, opts map[string]string) error {
	return r.doBackendOpts(ctx, app, http.MethodPut, opts)
}

func (r *apiRouter) doBackendOpts(ctx context.Context, app router.App, method string, opts map[string]string) error {
	path := fmt.Sprintf("backend/%s", app.GetName())
	b, err := json.Marshal(addDefaultOpts(app, opts))
	if err != nil {
		return err
	}

	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return err
	}

	data := bytes.NewReader(b)
	_, statusCode, err := r.do(ctx, method, path, headers, data)
	if statusCode == http.StatusConflict {
		return router.ErrBackendExists
	}
	if statusCode == http.StatusNotFound {
		return router.ErrBackendNotFound
	}
	return err
}

func (r *apiRouter) RemoveBackend(ctx context.Context, app router.App) (err error) {
	path := fmt.Sprintf("backend/%s", app.GetName())
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return err
	}
	data, statusCode, err := r.do(ctx, http.MethodDelete, path, headers, nil)
	switch statusCode {
	case http.StatusNotFound:
		return router.ErrBackendNotFound
	case http.StatusBadRequest:
		if strings.Contains(string(data), router.ErrBackendSwapped.Error()) {
			return router.ErrBackendSwapped
		}
	}
	return err
}

func (r *apiRouter) AddRoutes(ctx context.Context, app router.App, addresses []*url.URL) (err error) {
	return r.doRoutes(ctx, app, addresses, "")
}

func (r *apiRouter) RemoveRoutes(ctx context.Context, app router.App, addresses []*url.URL) (err error) {
	return r.doRoutes(ctx, app, addresses, "/remove")
}

func (r *apiRouter) doRoutes(ctx context.Context, app router.App, addresses []*url.URL, suffix string) error {
	req := &routesReq{}
	req.Addresses = make([]string, len(addresses))
	for i := range addresses {
		req.Addresses[i] = addresses[i].String()
	}
	return r.doRoutesReq(ctx, app, req, suffix)
}

func (r *apiRouter) doRoutesReq(ctx context.Context, app router.App, req *routesReq, suffix string) error {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return err
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	body := bytes.NewReader(data)
	path := fmt.Sprintf("backend/%s/routes%s", backendName, suffix)
	_, statusCode, err := r.do(ctx, http.MethodPost, path, headers, body)
	if statusCode == http.StatusNotFound {
		return router.ErrBackendNotFound
	}
	return err
}

func (r *apiRouter) Routes(ctx context.Context, app router.App) (result []*url.URL, err error) {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("backend/%s/routes", backendName)
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return nil, err
	}
	data, statusCode, err := r.do(ctx, http.MethodGet, path, headers, nil)
	if statusCode == http.StatusNotFound {
		return nil, router.ErrBackendNotFound
	}
	if err != nil {
		return nil, err
	}
	req := &routesPrefixReq{}
	err = json.Unmarshal(data, req)
	if err != nil {
		return nil, err
	}
	result = []*url.URL{}
	for _, addr := range req.Addresses {
		u, err := url.Parse(addr)
		if err != nil {
			return nil, errors.Errorf("failed to parse url %s: %s", addr, err)
		}
		result = append(result, u)
	}
	return result, nil
}

func (r *apiRouter) Addr(ctx context.Context, app router.App) (addr string, err error) {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return "", err
	}
	path := fmt.Sprintf("backend/%s", backendName)
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return "", err
	}
	data, code, err := r.do(ctx, http.MethodGet, path, headers, nil)
	if err != nil {
		if code == http.StatusNotFound {
			return "", router.ErrBackendNotFound
		}
		return "", err
	}
	resp := &backendResp{}
	err = json.Unmarshal(data, resp)
	return resp.Address, err
}

func (r *apiRouter) Swap(ctx context.Context, app1 router.App, app2 router.App, cnameOnly bool) (err error) {
	path := fmt.Sprintf("backend/%s/swap", app1.GetName())
	data, err := json.Marshal(swapReq{Target: app2.GetName(), CnameOnly: cnameOnly})
	if err != nil {
		return err
	}
	body := bytes.NewReader(data)

	if r.multiCluster && app1.GetPool() != app2.GetPool() {
		return router.ErrSwapAmongDifferentClusters
	}

	headers, err := r.getExtraHeadersFromApp(ctx, app1)
	if err != nil {
		return err
	}
	_, code, err := r.do(ctx, http.MethodPost, path, headers, body)
	if code == http.StatusNotFound {
		return router.ErrBackendNotFound
	}
	if err != nil {
		return err
	}
	return router.Swap(ctx, r.supIface, app1, app2, cnameOnly)
}

func (r *apiRouter) StartupMessage() (string, error) {
	return fmt.Sprintf("api router %q with endpoint %q", r.routerName, r.endpoint), nil
}

func (r *apiRouter) HealthCheck(ctx context.Context) error {
	data, code, err := r.do(ctx, http.MethodGet, "healthcheck", nil, nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return errors.Errorf("invalid status code %d from healthcheck %q: %s", code, r.endpoint+"/healthcheck", data)
	}
	return nil
}

func (r *apiRouter) checkSupports(ctx context.Context, feature string) (bool, error) {
	path := fmt.Sprintf("support/%s", feature)
	data, statusCode, err := r.do(ctx, http.MethodGet, path, nil, nil)
	switch statusCode {
	case http.StatusNotFound:
		return false, nil
	case http.StatusOK:
		return true, nil
	}
	return false, errors.Errorf("failed to check support for %s: %s - %s - %d", feature, err, data, statusCode)
}

func (r *apiRouter) do(ctx context.Context, method, path string, headers http.Header, body io.Reader) (data []byte, code int, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	url := fmt.Sprintf("%s/%s", strings.TrimRight(r.endpoint, "/"), strings.TrimLeft(path, "/"))
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	for k, values := range headers {
		for _, value := range values {
			req.Header.Add(k, value)
		}
	}

	for k, values := range r.headers {
		for _, value := range values {
			req.Header.Add(k, value)
		}
	}

	if ctx != nil {
		req = req.WithContext(ctx)
	}
	resp, err := r.client.Do(req)
	if r.debug {
		bodyData, _ := ioutil.ReadAll(body)
		if err == nil {
			code = resp.StatusCode
		}
		log.Debugf("%s %s %s %s: %d", r.routerName, method, url, string(bodyData), code)
	}
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	code = resp.StatusCode
	data, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return data, code, errors.Errorf("failed to read response body for %s: %s", url, err)
	}
	if resp.StatusCode >= 300 {
		return data, code, errors.Errorf("failed to request %s - %d - %s", url, code, data)
	}
	return data, code, nil
}

func (r *apiRouterWithCnameSupport) SetCName(ctx context.Context, cname string, app router.App) error {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return err
	}
	_, code, err := r.do(ctx, http.MethodPost, fmt.Sprintf("backend/%s/cname/%s", backendName, cname), headers, nil)
	switch code {
	case http.StatusBadRequest:
		return router.ErrCNameNotAllowed
	case http.StatusNotFound:
		return router.ErrBackendNotFound
	case http.StatusConflict:
		return router.ErrCNameExists
	}
	return err
}

func (r *apiRouterWithCnameSupport) UnsetCName(ctx context.Context, cname string, app router.App) error {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return err
	}
	data, code, err := r.do(ctx, http.MethodDelete, fmt.Sprintf("backend/%s/cname/%s", backendName, cname), headers, nil)
	switch code {
	case http.StatusNotFound:
		return router.ErrBackendNotFound
	case http.StatusBadRequest:
		if strings.Contains(string(data), router.ErrCNameNotFound.Error()) {
			return router.ErrCNameNotFound
		}
	}
	return err
}

func (r *apiRouterWithCnameSupport) CNames(ctx context.Context, app router.App) ([]*url.URL, error) {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return nil, err
	}
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return nil, err
	}
	data, code, err := r.do(ctx, http.MethodGet, fmt.Sprintf("backend/%s/cname", backendName), headers, nil)
	if code == http.StatusNotFound {
		return nil, router.ErrBackendNotFound
	}
	if err != nil {
		return nil, err
	}
	var resp cnamesResp
	err = json.Unmarshal(data, &resp)
	if err != nil {
		return nil, err
	}
	var urls []*url.URL
	for _, addr := range resp.Cnames {
		urls = append(urls, &url.URL{Host: addr})
	}
	return urls, nil
}

func (r *apiRouterWithTLSSupport) AddCertificate(ctx context.Context, app router.App, cname, certificate, key string) error {
	cert := certData{Certificate: certificate, Key: key}
	b, err := json.Marshal(&cert)
	if err != nil {
		return err
	}
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return err
	}
	_, _, err = r.do(ctx, http.MethodPut, fmt.Sprintf("backend/%s/certificate/%s", app.GetName(), cname), headers, bytes.NewReader(b))
	return err
}

func (r *apiRouterWithTLSSupport) RemoveCertificate(ctx context.Context, app router.App, cname string) error {
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return err
	}
	_, code, err := r.do(ctx, http.MethodDelete, fmt.Sprintf("backend/%s/certificate/%s", app.GetName(), cname), headers, nil)
	if code == http.StatusNotFound {
		return router.ErrCertificateNotFound
	}
	return err
}

func (r *apiRouterWithTLSSupport) GetCertificate(ctx context.Context, app router.App, cname string) (string, error) {
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return "", err
	}
	data, code, err := r.do(ctx, http.MethodGet, fmt.Sprintf("backend/%s/certificate/%s", app.GetName(), cname), headers, nil)
	switch code {
	case http.StatusNotFound:
		return "", router.ErrCertificateNotFound
	case http.StatusOK:
		var cert certData
		errJSON := json.Unmarshal(data, &cert)
		if errJSON != nil {
			return "", errJSON
		}
		return cert.Certificate, nil
	}
	return "", err
}

func (r *apiRouterWithHealthcheckSupport) SetHealthcheck(ctx context.Context, app router.App, data routerTypes.HealthcheckData) error {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return err
	}
	_, code, err := r.do(ctx, http.MethodPut, fmt.Sprintf("backend/%s/healthcheck", backendName), headers, bytes.NewReader(b))
	if code == http.StatusNotFound {
		return router.ErrBackendNotFound
	}
	return err
}

func (r *apiRouterWithInfo) GetInfo(ctx context.Context) (map[string]string, error) {
	data, _, err := r.do(ctx, http.MethodGet, "info", nil, nil)
	if err != nil {
		return nil, err
	}
	var result map[string]string
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *apiRouterWithStatus) GetBackendStatus(ctx context.Context, app router.App) (router.BackendStatus, string, error) {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return "", "", err
	}
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return "", "", err
	}
	data, _, err := r.do(ctx, http.MethodGet, fmt.Sprintf("backend/%s/status", backendName), headers, nil)
	if err != nil {
		return "", "", err
	}
	var status statusResp
	err = json.Unmarshal(data, &status)
	if err != nil {
		return "", "", err
	}
	return status.Status, status.Detail, nil
}

func (r *apiRouterWithPrefix) Addresses(ctx context.Context, app router.App) (addrs []string, err error) {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("backend/%s", backendName)
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return nil, err
	}
	data, code, err := r.do(ctx, http.MethodGet, path, headers, nil)
	if err != nil {
		if code == http.StatusNotFound {
			return nil, router.ErrBackendNotFound
		}
		return nil, err
	}
	resp := &backendResp{}
	err = json.Unmarshal(data, resp)
	return resp.Addresses, err
}

func (r *apiRouterWithPrefix) RoutesPrefix(ctx context.Context, app router.App) ([]appTypes.RoutableAddresses, error) {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("backend/%s/routes", backendName)
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return nil, err
	}
	data, statusCode, err := r.do(ctx, http.MethodGet, path, headers, nil)
	if statusCode == http.StatusNotFound {
		return nil, router.ErrBackendNotFound
	}
	if err != nil {
		return nil, err
	}
	var req routesPrefixReq
	err = json.Unmarshal(data, &req)
	if err != nil {
		return nil, err
	}
	var result []appTypes.RoutableAddresses
	for _, addrData := range req.AddressesWithPrefix {
		urls := []*url.URL{}
		for _, addr := range addrData.Addresses {
			u, err := url.Parse(addr)
			if err != nil {
				return nil, errors.Errorf("failed to parse url %s: %s", addr, err)
			}
			urls = append(urls, u)
		}
		result = append(result, appTypes.RoutableAddresses{
			Prefix:    addrData.Prefix,
			Addresses: urls,
			ExtraData: addrData.ExtraData,
		})
	}
	return result, nil
}

func (r *apiRouterWithPrefix) AddRoutesPrefix(ctx context.Context, app router.App, addresses appTypes.RoutableAddresses, sync bool) error {
	return r.doRoutesPrefix(ctx, app, addresses, "")
}

func (r *apiRouterWithPrefix) RemoveRoutesPrefix(ctx context.Context, app router.App, addresses appTypes.RoutableAddresses, sync bool) error {
	return r.doRoutesPrefix(ctx, app, addresses, "/remove")
}

func (r *apiRouter) doRoutesPrefix(ctx context.Context, app router.App, addresses appTypes.RoutableAddresses, suffix string) error {
	req := &routesReq{
		Prefix:    addresses.Prefix,
		ExtraData: addresses.ExtraData,
	}
	req.Addresses = make([]string, len(addresses.Addresses))
	for i := range addresses.Addresses {
		req.Addresses[i] = addresses.Addresses[i].String()
	}
	return r.doRoutesReq(ctx, app, req, suffix)
}

func addDefaultOpts(app router.App, opts map[string]string) map[string]interface{} {
	mergedOpts := make(map[string]interface{})
	for k, v := range opts {
		mergedOpts[k] = v
	}
	prefix := "tsuru.io/"
	mergedOpts[prefix+"app-pool"] = app.GetPool()
	mergedOpts[prefix+"app-teamowner"] = app.GetTeamOwner()
	mergedOpts[prefix+"app-teams"] = app.GetTeamsName()
	return mergedOpts
}

func (r *apiRouter) checkAllCapabilities(ctx context.Context) map[capability]bool {
	mu := sync.Mutex{}
	supports := map[capability]bool{}
	wg := sync.WaitGroup{}
	for _, cap := range allCaps {
		wg.Add(1)
		cap := cap
		go func() {
			defer wg.Done()
			result, err := r.checkSupports(ctx, string(cap))
			if err != nil {
				log.Errorf("failed to fetch %q support from router %q: %s", cap, r.routerName, err)
			}
			mu.Lock()
			defer mu.Unlock()
			supports[cap] = result
		}()
	}
	wg.Wait()
	return supports
}

func (r *apiRouter) getExtraHeadersFromApp(ctx context.Context, app router.App) (http.Header, error) {
	if !r.multiCluster {
		return http.Header{}, nil
	}

	poolName := app.GetPool()
	return poolMultiCluster.Header(ctx, poolName, nil)
}

func headersFromConfig(config router.ConfigGetter) (http.Header, error) {
	headers, _ := config.Get("headers")
	headerResult := make(http.Header)
	if headers != nil {
		h, ok := headers.(map[interface{}]interface{})
		if !ok {
			return nil, errors.Errorf("invalid header configuration: %v", headers)
		}

		for k, v := range h {
			_, isStr := v.(string)
			_, isSlice := v.([]interface{})
			if !isStr && !isSlice {
				return nil, errors.Errorf("invalid header configuration at key: %s. Expected string or array of strings got %v", k, v)
			}
			valueStr, strErr := config.GetString(fmt.Sprintf("headers:%s", k))
			valueList, listErr := config.GetList(fmt.Sprintf("headers:%s", k))

			if strErr == nil {
				headerResult.Add(fmt.Sprint(k), valueStr)
			}

			if listErr == nil {
				for _, v := range valueList {
					headerResult.Add(fmt.Sprint(k), v)
				}
			}
		}
	}

	return headerResult, nil
}
