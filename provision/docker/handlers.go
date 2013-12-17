// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
)

// AddNodeHandler calls cluster.Register registering a node into it.
func AddNodeHandler(w http.ResponseWriter, r *http.Request) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	var scheduler segregatedScheduler
	return scheduler.Register(params)
}

func RemoveNodeHandler(w http.ResponseWriter, r *http.Request) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	var scheduler segregatedScheduler
	return scheduler.Unregister(params)
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
