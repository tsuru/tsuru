// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"github.com/globocom/tsuru/api"
	"github.com/globocom/tsuru/auth"
	"io"
	"io/ioutil"
	"net/http"
)

func init() {
	api.RegisterAdminHandler("/node/add", "POST", api.Handler(addNodeHandler))
	api.RegisterAdminHandler("/node/remove", "DELETE", api.Handler(removeNodeHandler))
	api.RegisterHandler("/node/", "GET", api.AdminRequiredHandler(listNodeHandler))
	api.RegisterHandler("/node/:address/containers", "GET", api.AdminRequiredHandler(listContainersHandler))
}

// addNodeHandler calls scheduler.Register to registering a node into it.
func addNodeHandler(w http.ResponseWriter, r *http.Request) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	return dockerCluster().Register(params)
}

// removeNodeHandler calls scheduler.Unregister to unregistering a node into it.
func removeNodeHandler(w http.ResponseWriter, r *http.Request) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	return dockerCluster().Unregister(params)
}

//listNodeHandler call scheduler.Nodes to list all nodes into it.
func listNodeHandler(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	node_list, err := dockerCluster().Nodes()
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(node_list)
}

func listContainersHandler(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	n := r.URL.Query().Get(":address")
	container_list, err := ListContainersByNode(n)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(container_list)
}

func unmarshal(body io.ReadCloser) (map[string]string, error) {
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, err
	}
	params := map[string]string{}
	err = json.Unmarshal(b, &params)
	if err != nil {
		return nil, err
	}
	return params, nil
}
