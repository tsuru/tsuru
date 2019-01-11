// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

// title: create provisioner cluster
// path: /provisioner/clusters
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Pool does not exist
//   409: Cluster already exists
func createCluster(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowed := permission.Check(t, permission.PermClusterCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	var provCluster provTypes.Cluster
	err = ParseInput(r, &provCluster)
	if err != nil {
		return err
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeCluster, Value: provCluster.Name},
		Kind:       permission.PermClusterCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermClusterReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	_, err = servicemanager.Cluster.FindByName(provCluster.Name)
	if err == nil {
		return &tsuruErrors.HTTP{
			Code:    http.StatusConflict,
			Message: "cluster already exists",
		}
	}
	for _, poolName := range provCluster.Pools {
		_, err = pool.GetPoolByName(poolName)
		if err != nil {
			if err == pool.ErrPoolNotFound {
				return &tsuruErrors.HTTP{
					Code:    http.StatusNotFound,
					Message: err.Error(),
				}
			}
			return err
		}
	}
	err = servicemanager.Cluster.Create(provCluster)
	if err != nil {
		return errors.WithStack(err)
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

// title: update provisioner cluster
// path: /provisioner/clusters/{name}
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Cluster not found
func updateCluster(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowed := permission.Check(t, permission.PermClusterUpdate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	var provCluster provTypes.Cluster
	err = ParseInput(r, &provCluster)
	provCluster.Name = r.URL.Query().Get(":name")
	if err != nil {
		return err
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeCluster, Value: provCluster.Name},
		Kind:       permission.PermClusterUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermClusterReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	_, err = servicemanager.Cluster.FindByName(provCluster.Name)
	if err != nil {
		if err == provTypes.ErrClusterNotFound {
			return &tsuruErrors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return err
	}
	for _, poolName := range provCluster.Pools {
		_, err = pool.GetPoolByName(poolName)
		if err != nil {
			if err == pool.ErrPoolNotFound {
				return &tsuruErrors.HTTP{
					Code:    http.StatusBadRequest,
					Message: err.Error(),
				}
			}
			return err
		}
	}
	err = servicemanager.Cluster.Update(provCluster)
	if err != nil {
		return errors.WithStack(err)
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

// title: list provisioner clusters
// path: /provisioner/clusters
// method: GET
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//   200: Ok
//   204: No Content
//   401: Unauthorized
func listClusters(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowed := permission.Check(t, permission.PermClusterRead)
	if !allowed {
		return permission.ErrUnauthorized
	}
	clusters, err := servicemanager.Cluster.List()
	if err != nil {
		if err == provTypes.ErrNoCluster {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		return err
	}
	for i := range clusters {
		clusters[i].ClientKey = nil
	}
	return json.NewEncoder(w).Encode(clusters)
}

// title: delete provisioner cluster
// path: /provisioner/clusters/{name}
// method: DELETE
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
//   404: Cluster not found
func deleteCluster(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowed := permission.Check(t, permission.PermClusterDelete)
	if !allowed {
		return permission.ErrUnauthorized
	}

	clusterName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeCluster, Value: clusterName},
		Kind:       permission.PermClusterDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermClusterReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = servicemanager.Cluster.Delete(provTypes.Cluster{Name: clusterName})
	if err != nil {
		if errors.Cause(err) == provTypes.ErrClusterNotFound {
			return &tsuruErrors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return err
	}
	return nil
}
