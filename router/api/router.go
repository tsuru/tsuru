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

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/router"
)

const routerType = "api"

type apiRouter struct {
	routerName string
	endpoint   string
	client     *http.Client
}

type routesReq struct {
	Addresses []string `json:"addresses"`
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
	return &apiRouter{
		routerName: routerName,
		endpoint:   endpoint,
		client:     net.Dial5Full60ClientNoKeepAlive,
	}, nil
}

func (r *apiRouter) AddBackend(name string) (err error) {
	path := fmt.Sprintf("backend/%s", name)
	_, statusCode, err := r.do(http.MethodPost, path, nil)
	if statusCode == http.StatusConflict {
		return router.ErrBackendExists
	}
	return err
}

func (r *apiRouter) RemoveBackend(name string) (err error) {
	path := fmt.Sprintf("backend/%s", name)
	_, statusCode, err := r.do(http.MethodDelete, path, nil)
	switch statusCode {
	case http.StatusNotFound:
		return router.ErrBackendNotFound
	case http.StatusForbidden:
		return router.ErrBackendSwapped
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
