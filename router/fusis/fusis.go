// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fusis

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tsuru/config"
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
}

func init() {
	router.Register(routerType, createRouter)
}

func createRouter(routerName, configPrefix string) (router.Router, error) {
	apiUrl, err := config.GetString(configPrefix + ":api-url")
	if err != nil {
		return nil, err
	}
	r := fusisRouter{
		apiUrl:    apiUrl,
		proto:     "tcp",
		port:      80,
		scheduler: "rr",
		mode:      "nat",
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
	if r.Debug {
		bodyData = buf.String()
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	rsp, err := tsuruNet.Dial5Full60Client.Do(req)
	if true {
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
	data := map[string]interface{}{
		"Host": host,
		"Port": strconv.ParseInt(port, 10, 16),
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
	// TODO
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
func (r *fusisRouter) Addr(name string) (string, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return "", err
	}
	// TODO
	return "", nil
}
func (r *fusisRouter) Swap(string, string) error {
	return router.Swap(r, backend1, backend2)
}
func (r *fusisRouter) Routes(name string) ([]*url.URL, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	// TODO
	return nil, nil
}
