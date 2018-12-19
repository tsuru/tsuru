// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/install"
	"github.com/tsuru/tsuru/permission"
)

// title: add install host
// path: /install/hosts
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//   201: Host added
//   401: Unauthorized
func installHostAdd(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowed := permission.Check(t, permission.PermInstallManage)
	if !allowed {
		return permission.ErrUnauthorized
	}
	var host *install.Host
	err = ParseInput(r, &host)
	if err != nil {
		return err
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeInstallHost, Value: host.Name},
		Kind:       permission.PermInstallManage,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermInstallManage),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	var rawDriver map[string]interface{}
	err = json.Unmarshal([]byte(r.Form.Get("driver")), &rawDriver)
	if err != nil {
		return err
	}
	host.Driver = rawDriver
	err = install.AddHost(host)
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return nil
}

// title: install host info
// path: /install/hosts/{name}
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
//   404: Not Found
func installHostInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermInstallManage)
	if !allowed {
		return permission.ErrUnauthorized
	}
	host, err := install.GetHostByName(r.URL.Query().Get(":name"))
	if errNf, ok := err.(*install.ErrHostNotFound); ok {
		return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("Host %s not found.", errNf.Name)}
	}
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(&host)
}

// title: list install hosts
// path: /install/hosts
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
func installHostList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermInstallManage)
	if !allowed {
		return permission.ErrUnauthorized
	}
	hosts, err := install.ListHosts()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(&hosts)
}
