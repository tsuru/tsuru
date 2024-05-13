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
	"net/http"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	poolMultiCluster "github.com/tsuru/tsuru/provision/pool/multicluster"
	"github.com/tsuru/tsuru/router"
)

//go:generate bash -c "rm -f routeriface.go && go run ./generator/combinations.go -o routeriface.go"

const routerType = "api"

var (
	_ router.Router    = &apiRouter{}
	_ router.TLSRouter = &apiRouterWithTLSSupport{}
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

type apiRouterWithTLSSupport struct{ *apiRouter }

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

type capability string

var (
	capTLS = capability("tls")

	allCaps = []capability{capTLS}
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

func (r *apiRouter) GetType() string {
	return routerType
}

func (r *apiRouter) EnsureBackend(ctx context.Context, app router.App, o router.EnsureBackendOpts) error {
	path := fmt.Sprintf("backend/%s", app.GetName())

	o.Opts = addDefaultOpts(app, o.Opts)

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(o)
	if err != nil {
		return err
	}

	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return err
	}

	headers.Set("Content-Type", "application/json; router=v2")
	_, _, err = r.do(ctx, http.MethodPut, path, headers, &buf)
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

func (r *apiRouter) Addresses(ctx context.Context, app router.App) (addrs []string, err error) {
	backendName := app.GetName()
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
	if headers.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

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
		bodyData, _ := io.ReadAll(body)
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
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return data, code, errors.Errorf("failed to read response body for %s: %s", url, err)
	}
	if resp.StatusCode >= 300 {
		return data, code, errors.Errorf("failed to request %s - %d - %s", url, code, data)
	}
	return data, code, nil
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

func (r *apiRouter) GetInfo(ctx context.Context) (map[string]string, error) {
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

func (r *apiRouter) GetBackendStatus(ctx context.Context, app router.App) (router.RouterBackendStatus, error) {
	rsp := router.RouterBackendStatus{}
	backendName := app.GetName()
	headers, err := r.getExtraHeadersFromApp(ctx, app)
	if err != nil {
		return rsp, err
	}

	data, _, err := r.do(ctx, http.MethodGet, fmt.Sprintf("backend/%s/status", backendName), headers, nil)
	if err != nil {
		return rsp, err
	}
	err = json.Unmarshal(data, &rsp)
	if err != nil {
		return rsp, err
	}
	return rsp, nil
}

func addDefaultOpts(app router.App, opts map[string]interface{}) map[string]interface{} {
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
