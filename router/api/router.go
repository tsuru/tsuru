// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"strconv"

	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/router"
)

const routerType = "api"

var (
	_ router.OptsRouter              = &apiRouter{}
	_ router.Router                  = &apiRouter{}
	_ router.MessageRouter           = &apiRouter{}
	_ router.HealthChecker           = &apiRouter{}
	_ router.TLSRouter               = &apiRouterWithTLSSupport{}
	_ router.CNameRouter             = &apiRouterWithCnameSupport{}
	_ router.CustomHealthcheckRouter = &apiRouterWithHealthcheckSupport{}
)

type apiRouter struct {
	routerName string
	endpoint   string
	headers    map[string]string
	client     *http.Client
	debug      bool
}

type apiRouterWithCnameSupport struct{ *apiRouter }

type apiRouterWithTLSSupport struct{ *apiRouter }

type apiRouterWithHealthcheckSupport struct{ *apiRouter }

type routesReq struct {
	Addresses []string `json:"addresses"`
}

type cnamesResp struct {
	Cnames []string `json:"cnames"`
}

type certData struct {
	Certificate string `json:"certificate"`
	Key         string `json:"key"`
}

type backendResp struct {
	Address string `json:"address"`
}

func init() {
	router.Register(routerType, createRouter)
}

func createRouter(routerName, configPrefix string) (router.Router, error) {
	endpoint, err := config.GetString(configPrefix + ":api-url")
	if err != nil {
		return nil, err
	}
	debug, _ := config.GetBool(configPrefix + ":debug")
	headers, _ := config.Get(configPrefix + ":headers")
	headerMap := make(map[string]string)
	if headers != nil {
		h, ok := headers.(map[interface{}]interface{})
		if !ok {
			return nil, errors.Errorf("invalid header configuration: %v", headers)
		}
		for k, v := range h {
			k, okK := k.(string)
			v, okV := v.(string)
			if !okK || !okV {
				return nil, errors.Errorf("invalid header configuration: %v. Expected string got %s and %s", headers, k, v)
			}
			headerMap[k] = v
		}
	}
	baseRouter := &apiRouter{
		routerName: routerName,
		endpoint:   endpoint,
		client:     net.Dial5Full60ClientNoKeepAlive,
		debug:      debug,
		headers:    headerMap,
	}
	cnameAPI := &apiRouterWithCnameSupport{baseRouter}
	tlsAPI := &apiRouterWithTLSSupport{baseRouter}
	hcAPI := &apiRouterWithHealthcheckSupport{baseRouter}
	ifMap := map[[3]bool]router.Router{
		{true, false, false}: cnameAPI,
		{false, true, false}: tlsAPI,
		{false, false, true}: hcAPI,
		{true, true, false}: &struct {
			router.CNameRouter
			router.TLSRouter
		}{cnameAPI, tlsAPI},
		{true, false, true}: &struct {
			router.CNameRouter
			router.CustomHealthcheckRouter
		}{cnameAPI, hcAPI},
		{false, true, true}: &struct {
			*apiRouter
			router.TLSRouter
			router.CustomHealthcheckRouter
		}{baseRouter, tlsAPI, hcAPI},
		{true, true, true}: &struct {
			router.CNameRouter
			router.TLSRouter
			router.CustomHealthcheckRouter
		}{cnameAPI, tlsAPI, hcAPI},
	}
	var supports [3]bool
	for i, s := range []string{"cname", "tls", "healthcheck"} {
		var err error
		supports[i], err = baseRouter.checkSupports(s)
		if err != nil {
			log.Errorf("failed to fetch %q support from router %q: %s", s, routerName, err)
		}
	}
	if r, ok := ifMap[supports]; ok {
		return r, nil
	}
	return baseRouter, nil
}

func (r *apiRouter) AddBackend(name string) (err error) {
	return r.AddBackendOpts(name, nil)
}

func (r *apiRouter) AddBackendOpts(name string, opts map[string]string) error {
	err := r.doBackendOpts(name, http.MethodPost, opts)
	if err != nil {
		return err
	}
	return router.Store(name, name, routerType)
}

func (r *apiRouter) UpdateBackendOpts(name string, opts map[string]string) error {
	return r.doBackendOpts(name, http.MethodPut, opts)
}

func (r *apiRouter) doBackendOpts(name, method string, opts map[string]string) error {
	path := fmt.Sprintf("backend/%s", name)
	b, err := json.Marshal(opts)
	if err != nil {
		return err
	}
	data := bytes.NewReader(b)
	_, statusCode, err := r.do(method, path, data)
	if statusCode == http.StatusConflict {
		return router.ErrBackendExists
	}
	if statusCode == http.StatusNotFound {
		return router.ErrBackendNotFound
	}
	return err
}

func (r *apiRouter) RemoveBackend(name string) (err error) {
	path := fmt.Sprintf("backend/%s", name)
	data, statusCode, err := r.do(http.MethodDelete, path, nil)
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

func (r *apiRouter) AddRoutes(name string, addresses []*url.URL) (err error) {
	return r.doRoutes(name, addresses, "")
}

func (r *apiRouter) RemoveRoutes(name string, addresses []*url.URL) (err error) {
	return r.doRoutes(name, addresses, "/remove")
}

func (r *apiRouter) doRoutes(name string, addresses []*url.URL, suffix string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	req := &routesReq{}
	req.Addresses = make([]string, len(addresses))
	for i := range addresses {
		req.Addresses[i] = addresses[i].String()
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	body := bytes.NewReader(data)
	path := fmt.Sprintf("backend/%s/routes%s", backendName, suffix)
	_, statusCode, err := r.do(http.MethodPost, path, body)
	if statusCode == http.StatusNotFound {
		return router.ErrBackendNotFound
	}
	return err
}

func (r *apiRouter) Routes(name string) (result []*url.URL, err error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("backend/%s/routes", backendName)
	data, statusCode, err := r.do(http.MethodGet, path, nil)
	if statusCode == http.StatusNotFound {
		return nil, router.ErrBackendNotFound
	}
	if err != nil {
		return nil, err
	}
	req := &routesReq{}
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

func (r *apiRouter) Addr(name string) (addr string, err error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return "", err
	}
	path := fmt.Sprintf("backend/%s", backendName)
	data, code, err := r.do(http.MethodGet, path, nil)
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

func (r *apiRouter) Swap(backend1 string, backend2 string, cnameOnly bool) (err error) {
	path := fmt.Sprintf("backend/%s/swap?target=%s&cnameOnly=%s", backend1, backend2, strconv.FormatBool(cnameOnly))
	_, code, err := r.do(http.MethodPost, path, nil)
	if code == http.StatusNotFound {
		return router.ErrBackendNotFound
	}
	if err != nil {
		return err
	}
	return router.Swap(r, backend1, backend2, cnameOnly)
}

func (r *apiRouter) StartupMessage() (string, error) {
	return fmt.Sprintf("api router %q with endpoint %q", r.routerName, r.endpoint), nil
}

func (r *apiRouter) HealthCheck() error {
	data, code, err := r.do(http.MethodGet, "healthcheck", nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return errors.Errorf("invalid status code %d from healthcheck %q: %s", code, r.endpoint+"/healthcheck", data)
	}
	return nil
}

func (r *apiRouter) checkSupports(feature string) (bool, error) {
	path := fmt.Sprintf("support/%s", feature)
	data, statusCode, err := r.do(http.MethodGet, path, nil)
	switch statusCode {
	case http.StatusNotFound:
		return false, nil
	case http.StatusOK:
		return true, nil
	}
	return false, errors.Errorf("failed to check support for %s: %s - %s - %d", feature, err, data, statusCode)
}

func (r *apiRouter) do(method, path string, body io.Reader) (data []byte, code int, err error) {
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
	for k, v := range r.headers {
		req.Header.Set(k, v)
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

func (r *apiRouterWithCnameSupport) SetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	_, code, err := r.do(http.MethodPost, fmt.Sprintf("backend/%s/cname/%s", backendName, cname), nil)
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

func (r *apiRouterWithCnameSupport) UnsetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	data, code, err := r.do(http.MethodDelete, fmt.Sprintf("backend/%s/cname/%s", backendName, cname), nil)
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

func (r *apiRouterWithCnameSupport) CNames(name string) ([]*url.URL, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	data, code, err := r.do(http.MethodGet, fmt.Sprintf("backend/%s/cname", backendName), nil)
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

func (r *apiRouterWithTLSSupport) AddCertificate(cname, certificate, key string) error {
	cert := certData{Certificate: certificate, Key: key}
	b, err := json.Marshal(&cert)
	if err != nil {
		return err
	}
	_, _, err = r.do(http.MethodPut, fmt.Sprintf("certificate/%s", cname), bytes.NewReader(b))
	return err
}

func (r *apiRouterWithTLSSupport) RemoveCertificate(cname string) error {
	_, code, err := r.do(http.MethodDelete, fmt.Sprintf("certificate/%s", cname), nil)
	if code == http.StatusNotFound {
		return router.ErrCertificateNotFound
	}
	return err
}

func (r *apiRouterWithTLSSupport) GetCertificate(cname string) (string, error) {
	data, code, err := r.do(http.MethodGet, fmt.Sprintf("certificate/%s", cname), nil)
	switch code {
	case http.StatusNotFound:
		return "", router.ErrCertificateNotFound
	case http.StatusOK:
		var cert string
		errJSON := json.Unmarshal(data, &cert)
		if errJSON != nil {
			return "", errJSON
		}
		return cert, nil
	}
	return "", err
}

func (r *apiRouterWithHealthcheckSupport) SetHealthcheck(name string, data router.HealthcheckData) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, code, err := r.do(http.MethodPut, fmt.Sprintf("backend/%s/healthcheck", backendName), bytes.NewReader(b))
	if code == http.StatusNotFound {
		return router.ErrBackendNotFound
	}
	return err
}
