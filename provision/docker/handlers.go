// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/tsuru/api"
	"io"
	"io/ioutil"
	"net/http"
)

func init() {
	api.RegisterHandler("/node/add", "POST", api.Handler(addNodeHandler))
	api.RegisterHandler("/node/remove", "DELETE", api.Handler(removeNodeHandler))
}

// AddNodeHandler calls scheduler.Register registering a node into it.
func addNodeHandler(w http.ResponseWriter, r *http.Request) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	scheduler := getScheduler()
	if r, ok := scheduler.(cluster.Registrable); ok {
		return r.Register(params)
	}
	return nil
}

func removeNodeHandler(w http.ResponseWriter, r *http.Request) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	scheduler := getScheduler()
	if r, ok := scheduler.(cluster.Registrable); ok {
		return r.Unregister(params)
	}
	return nil
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
