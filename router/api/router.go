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
	client     *http.Client
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
	endpoint, err := config.GetString(configPrefix + ":endpoint")
	if err != nil {
		return nil, err
	}
	baseRouter := &apiRouter{
		routerName: routerName,
		endpoint:   endpoint,
		client:     net.Dial5Full60ClientNoKeepAlive,
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
	path := fmt.Sprintf("backend/%s", name)
	b, err := json.Marshal(opts)
	if err != nil {
		return err
	}
	data := bytes.NewReader(b)
	_, statusCode, err := r.do(http.MethodPost, path, data)
	if statusCode == http.StatusConflict {
		return router.ErrBackendExists
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
		fallthrough
	default:
		return err
	}
}

func (r *apiRouter) AddRoute(name string, address *url.URL) error {
	return r.AddRoutes(name, []*url.URL{address})
}

func (r *apiRouter) AddRoutes(name string, addresses []*url.URL) (err error) {
	currRoutes, err := r.Routes(name)
	if err != nil {
		return err
	}
	routesMap := make(map[*url.URL]struct{})
	for i := range currRoutes {
		routesMap[currRoutes[i]] = struct{}{}
	}
	for i := range addresses {
		routesMap[addresses[i]] = struct{}{}
	}
	newAddresses := make([]*url.URL, len(routesMap))
	idx := 0
	for v := range routesMap {
		newAddresses[idx] = v
		idx++
	}
	return r.setRoutes(name, newAddresses)
}

func (r *apiRouter) RemoveRoute(name string, address *url.URL) (err error) {
	return r.RemoveRoutes(name, []*url.URL{address})
}

func (r *apiRouter) RemoveRoutes(name string, addresses []*url.URL) (err error) {
	currRoutes, err := r.Routes(name)
	if err != nil {
		return err
	}
	routesMap := make(map[url.URL]struct{})
	for i := range currRoutes {
		routesMap[*currRoutes[i]] = struct{}{}
	}
	for i := range addresses {
		delete(routesMap, *addresses[i])
	}
	newAddresses := make([]*url.URL, len(routesMap))
	idx := 0
	for v := range routesMap {
		newAddresses[idx] = &v
		idx++
	}
	return r.setRoutes(name, newAddresses)
}

func (r *apiRouter) Routes(name string) (result []*url.URL, err error) {
	path := fmt.Sprintf("backend/%s/routes", name)
	data, statusCode, err := r.do(http.MethodGet, path, nil)
	if statusCode == http.StatusNotFound {
		return nil, router.ErrBackendNotFound
	}
	req := &routesReq{}
	err = json.Unmarshal(data, req)
	if err != nil {
		return nil, err
	}
	for _, addr := range req.Addresses {
		u, err := url.Parse(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse url %s: %s", addr, err)
		}
		result = append(result, u)
	}
	return result, nil
}

func (r *apiRouter) Addr(name string) (addr string, err error) {
	path := fmt.Sprintf("backend/%s", name)
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
	return err
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
		return fmt.Errorf("invalid status code %d from healthcheck %q: %s", code, r.endpoint+"/healthcheck", data)
	}
	return nil
}

func (r *apiRouter) setRoutes(name string, addresses []*url.URL) (err error) {
	path := fmt.Sprintf("backend/%s/routes", name)
	req := &routesReq{}
	for _, addr := range addresses {
		req.Addresses = append(req.Addresses, addr.String())
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	body := bytes.NewReader(data)
	_, statusCode, err := r.do(http.MethodPut, path, body)
	switch statusCode {
	case http.StatusNotFound:
		return router.ErrBackendNotFound
	default:
		return err
	}
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
	return false, fmt.Errorf("failed to check support for %s: %s - %s - %d", feature, err, data, statusCode)
}

func (r *apiRouter) do(method, path string, body io.Reader) (data []byte, code int, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	url := fmt.Sprintf("%s/%s", r.endpoint, path)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	code = resp.StatusCode
	data, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return data, code, fmt.Errorf("failed to read response body for %s: %s", url, err)
	}
	if resp.StatusCode >= 400 {
		return data, code, fmt.Errorf("failed to request %s - %d - %s", url, code, data)
	}
	return data, code, nil
}

func (r *apiRouterWithCnameSupport) SetCName(cname, name string) error {
	_, code, err := r.do(http.MethodPost, fmt.Sprintf("backend/%s/cname/%s", name, cname), nil)
	switch code {
	case http.StatusNotFound:
		return router.ErrBackendNotFound
	case http.StatusConflict:
		return router.ErrCNameExists
	default:
		return err
	}
}

func (r *apiRouterWithCnameSupport) UnsetCName(cname, name string) error {
	data, code, err := r.do(http.MethodDelete, fmt.Sprintf("backend/%s/cname/%s", name, cname), nil)
	switch code {
	case http.StatusNotFound:
		return router.ErrBackendNotFound
	case http.StatusBadRequest:
		if strings.Contains(string(data), router.ErrCNameNotFound.Error()) {
			return router.ErrCNameNotFound
		}
		fallthrough
	default:
		return err
	}
}

func (r *apiRouterWithCnameSupport) CNames(name string) ([]*url.URL, error) {
	data, code, err := r.do(http.MethodGet, fmt.Sprintf("backend/%s/cname", name), nil)
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
		parsed, err := url.Parse(addr)
		if err != nil {
			return nil, err
		}
		urls = append(urls, parsed)
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
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, code, err := r.do(http.MethodPut, fmt.Sprintf("backend/%s/healthcheck", name), bytes.NewReader(b))
	if code == http.StatusNotFound {
		return router.ErrBackendNotFound
	}
	return err
}
