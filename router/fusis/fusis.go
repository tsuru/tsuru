// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fusis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/router"
)

const routerType = "fusis"

type fusisRouter struct {
	apiUrl    string
	proto     string
	port      uint16
	scheduler string
	mode      string
	debug     bool
}

func init() {
	router.Register(routerType, createRouter)
}

func createRouter(routerName, configPrefix string) (router.Router, error) {
	apiUrl, err := config.GetString(configPrefix + ":api-url")
	if err != nil {
		return nil, err
	}
	r := &fusisRouter{
		apiUrl:    apiUrl,
		proto:     "tcp",
		port:      80,
		scheduler: "rr",
		mode:      "nat",
		debug:     true,
	}
	return r, nil
}

func (r *fusisRouter) doRequest(method, path string, params interface{}) (*http.Response, error) {
	buf := bytes.Buffer{}
	if params != nil {
		err := json.NewEncoder(&buf).Encode(params)
		if err != nil {
			return nil, err
		}
	}
	url := fmt.Sprintf("%s/%s", strings.TrimRight(r.apiUrl, "/"), strings.TrimLeft(path, "/"))
	var bodyData string
	if r.debug {
		bodyData = buf.String()
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	rsp, err := tsuruNet.Dial5Full60Client.Do(req)
	if r.debug {
		var code int
		if err == nil {
			code = rsp.StatusCode
		}
		log.Debugf("request %s %s %s: %d", method, url, bodyData, code)
	}
	return rsp, err
}

func (r *fusisRouter) AddBackend(name string) error {
	rsp, err := r.doRequest("POST", "/services", map[string]interface{}{
		"Name":      name,
		"Port":      r.port,
		"Protocol":  r.proto,
		"Scheduler": r.scheduler,
	})
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		data, _ := ioutil.ReadAll(rsp.Body)
		return fmt.Errorf("invalid response %d: %s", rsp.StatusCode, string(data))
	}
	return router.Store(name, name, routerType)
}
func (r *fusisRouter) RemoveBackend(name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	rsp, err := r.doRequest("DELETE", "/services/"+backendName, nil)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusOK {
		data, _ := ioutil.ReadAll(rsp.Body)
		return fmt.Errorf("invalid response %d: %s", rsp.StatusCode, string(data))
	}
	return nil
}
func (r *fusisRouter) routeName(name string, address *url.URL) string {
	return fmt.Sprintf("%s_%s", name, address.String())
}

func (r *fusisRouter) AddRoute(name string, address *url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(address.Host)
	if err != nil {
		host = address.Host
		port = "80"
	}
	portInt, _ := strconv.ParseInt(port, 10, 16)
	data := map[string]interface{}{
		"Name": r.routeName(backendName, address),
		"Host": host,
		"Port": portInt,
		"Mode": r.mode,
	}
	rsp, err := r.doRequest("POST", fmt.Sprintf("/services/%s/destinations", backendName), data)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusOK {
		data, _ := ioutil.ReadAll(rsp.Body)
		return fmt.Errorf("invalid response %d: %s", rsp.StatusCode, string(data))
	}
	return nil
}
func (r *fusisRouter) AddRoutes(name string, addresses []*url.URL) error {
	for _, addr := range addresses {
		err := r.AddRoute(name, addr)
		if err != nil {
			return err
		}
	}
	return nil
}
func (r *fusisRouter) RemoveRoute(name string, address *url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	rsp, err := r.doRequest("DELETE", fmt.Sprintf("/services/%s/destinations/%s", backendName, r.routeName(backendName, address)), nil)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusOK {
		data, _ := ioutil.ReadAll(rsp.Body)
		return fmt.Errorf("invalid response %d: %s", rsp.StatusCode, string(data))
	}
	return nil
}
func (r *fusisRouter) RemoveRoutes(name string, addresses []*url.URL) error {
	for _, addr := range addresses {
		err := r.RemoveRoute(name, addr)
		if err != nil {
			return err
		}
	}
	return nil
}
func (r *fusisRouter) SetCName(cname, name string) error {
	return nil
}
func (r *fusisRouter) UnsetCName(cname, name string) error {
	return nil
}

type Service struct {
	Name         string
	Host         string
	Port         uint16
	Destinations []Destination
}

type Destination struct {
	Name      string
	Host      string
	Port      uint16
	Weight    int32
	Mode      string
	ServiceId string `json:"service_id"`
}

func (r *fusisRouter) findService(name string) (*Service, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	rsp, err := r.doRequest("GET", "/services", nil)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	data, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid response %d: %s", rsp.StatusCode, string(data))
	}
	var services []Service
	err = json.Unmarshal(data, &services)
	if err != nil {
		return nil, fmt.Errorf("unable unmarshal %q: %s", string(data), err)
	}
	var foundService *Service
	for i, s := range services {
		if s.Name == backendName {
			foundService = &services[i]
			break
		}
	}
	if foundService == nil {
		return nil, fmt.Errorf("service %s not found", backendName)
	}
	return foundService, nil
}

func (r *fusisRouter) Addr(name string) (string, error) {
	srv, err := r.findService(name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s", srv.Host, srv.Port), nil
}
func (r *fusisRouter) Swap(backend1 string, backend2 string) error {
	return router.Swap(r, backend1, backend2)
}
func (r *fusisRouter) Routes(name string) ([]*url.URL, error) {
	srv, err := r.findService(name)
	if err != nil {
		return nil, err
	}
	result := make([]*url.URL, len(srv.Destinations))
	for i, d := range srv.Destinations {
		var err error
		result[i], err = url.Parse(fmt.Sprintf("http://%s:%s", d.Host, d.Port))
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
