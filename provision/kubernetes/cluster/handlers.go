// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"encoding/json"
	"net/http"

	"github.com/ajg/form"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
)

var (
	targetTypeCluster = event.TargetType("cluster")
)

func init() {
	api.RegisterHandlerVersion("1.3", "/kubernetes/clusters", "POST", api.AuthorizationRequiredHandler(updateCluster))
	api.RegisterHandlerVersion("1.3", "/kubernetes/clusters", "GET", api.AuthorizationRequiredHandler(listClusters))
	api.RegisterHandlerVersion("1.3", "/kubernetes/clusters/{name}", "DELETE", api.AuthorizationRequiredHandler(deleteCluster))
}

// title: create or update kubernetes cluster
// path: /kubernetes/clusters
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   409: Cluster already exists
func updateCluster(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowed := permission.Check(t, permission.PermKubernetesClusterUpdate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	var cluster Cluster
	err = r.ParseForm()
	if err == nil {
		err = dec.DecodeValues(&cluster, r.Form)
	}
	if err != nil {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: targetTypeCluster, Value: cluster.Name},
		Kind:       permission.PermKubernetesClusterUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermKubernetesClusterReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = cluster.Save()
	if err != nil {
		if _, ok := errors.Cause(err).(*tsuruErrors.ValidationError); ok {
			return &tsuruErrors.HTTP{
				Code:    http.StatusBadRequest,
				Message: err.Error(),
			}
		}
		return errors.WithStack(err)
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

// title: list kubernetes clusters
// path: /kubernetes/clusters
// method: GET
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//   200: Ok
//   204: No Content
//   401: Unauthorized
func listClusters(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowed := permission.Check(t, permission.PermKubernetesClusterRead)
	if !allowed {
		return permission.ErrUnauthorized
	}
	clusters, err := AllClusters()
	if err != nil {
		if err == ErrNoCluster {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		return err
	}
	return json.NewEncoder(w).Encode(clusters)
}

// title: delete kubernetes cluster
// path: /kubernetes/clusters/{name}
// method: GET
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
//   404: Cluster not found
func deleteCluster(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowed := permission.Check(t, permission.PermKubernetesClusterDelete)
	if !allowed {
		return permission.ErrUnauthorized
	}
	r.ParseForm()
	clusterName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: targetTypeCluster, Value: clusterName},
		Kind:       permission.PermKubernetesClusterDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermKubernetesClusterReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = DeleteCluster(clusterName)
	if err != nil {
		if errors.Cause(err) == ErrClusterNotFound {
			return &tsuruErrors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return err
	}
	return nil
}
