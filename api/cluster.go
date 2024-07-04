// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	eventTypes "github.com/tsuru/tsuru/types/event"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

// title: create provisioner cluster
// path: /provisioner/clusters
// method: POST
// consume: application/json
// produce: application/x-json-stream
// responses:
//
//	200: Ok
//	400: Invalid data
//	401: Unauthorized
//	404: Pool does not exist
//	409: Cluster already exists
func createCluster(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	allowed := permission.Check(ctx, t, permission.PermClusterCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}

	err = deprecateFormContentType(r)
	if err != nil {
		return err
	}

	var provCluster provTypes.Cluster
	err = ParseJSON(r, &provCluster)
	if err != nil {
		return err
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeCluster, Value: provCluster.Name},
		Kind:       permission.PermClusterCreate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermClusterReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	_, err = servicemanager.Cluster.FindByName(ctx, provCluster.Name)
	if err == nil {
		return &tsuruErrors.HTTP{
			Code:    http.StatusConflict,
			Message: "cluster already exists",
		}
	}
	for _, poolName := range provCluster.Pools {
		_, err = pool.GetPoolByName(ctx, poolName)
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
	streamResponse := strings.HasPrefix(r.Header.Get("Accept"), "application/x-json-stream")
	if streamResponse {
		w.Header().Set("Content-Type", "application/x-json-stream")
		keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
		defer keepAliveWriter.Stop()
		writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
		evt.SetLogWriter(writer)
	}
	err = servicemanager.Cluster.Create(ctx, provCluster)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// title: update provisioner cluster
// path: /provisioner/clusters/{name}
// method: POST
// consume: application/json
// produce: application/x-json-stream
// responses:
//
//	200: Ok
//	400: Invalid data
//	401: Unauthorized
//	404: Cluster not found
func updateCluster(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	allowed := permission.Check(ctx, t, permission.PermClusterUpdate)
	if !allowed {
		return permission.ErrUnauthorized
	}

	err = deprecateFormContentType(r)
	if err != nil {
		return err
	}

	var provCluster provTypes.Cluster
	err = ParseJSON(r, &provCluster)
	provCluster.Name = r.URL.Query().Get(":name")
	if err != nil {
		return err
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeCluster, Value: provCluster.Name},
		Kind:       permission.PermClusterUpdate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermClusterReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	_, err = servicemanager.Cluster.FindByName(ctx, provCluster.Name)
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
		_, err = pool.GetPoolByName(ctx, poolName)
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
	streamResponse := strings.HasPrefix(r.Header.Get("Accept"), "application/x-json-stream")
	if streamResponse {
		w.Header().Set("Content-Type", "application/x-json-stream")
		keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
		defer keepAliveWriter.Stop()
		writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
		evt.SetLogWriter(writer)
	}
	err = servicemanager.Cluster.Update(ctx, provCluster)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// title: list provisioner clusters
// path: /provisioner/clusters
// method: GET
// produce: application/json
// responses:
//
//	200: Ok
//	204: No Content
//	401: Unauthorized
func listClusters(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	allowed := permission.Check(ctx, t, permission.PermClusterRead)
	if !allowed {
		return permission.ErrUnauthorized
	}
	clusters, err := servicemanager.Cluster.List(ctx)
	if err != nil {
		if err == provTypes.ErrNoCluster {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		return err
	}
	admin := permission.Check(ctx, t, permission.PermClusterAdmin)
	if !admin {
		for i := range clusters {
			clusters[i].CleanUpSensitive()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(clusters)
}

// title: provisioner cluster info
// path: /provisioner/clusters/{name}
// method: GET
// produce: application/json
// responses:
//
//	200: Ok
//	401: Unauthorized
//	404: Cluster not found
func clusterInfo(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	allowed := permission.Check(ctx, t, permission.PermClusterRead)
	if !allowed {
		return permission.ErrUnauthorized
	}
	name := r.URL.Query().Get(":name")
	cluster, err := servicemanager.Cluster.FindByName(ctx, name)
	if err != nil {
		if err == provTypes.ErrClusterNotFound {
			return &tsuruErrors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(cluster)
}

// title: delete provisioner cluster
// path: /provisioner/clusters/{name}
// method: DELETE
// produce: application/x-json-stream
// responses:
//
//	200: Ok
//	401: Unauthorized
//	404: Cluster not found
func deleteCluster(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	allowed := permission.Check(ctx, t, permission.PermClusterDelete)
	if !allowed {
		return permission.ErrUnauthorized
	}

	clusterName := r.URL.Query().Get(":name")
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeCluster, Value: clusterName},
		Kind:       permission.PermClusterDelete,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermClusterReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	streamResponse := strings.HasPrefix(r.Header.Get("Accept"), "application/x-json-stream")
	if streamResponse {
		w.Header().Set("Content-Type", "application/x-json-stream")
		keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
		defer keepAliveWriter.Stop()
		writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
		evt.SetLogWriter(writer)
	}
	err = servicemanager.Cluster.Delete(ctx, provTypes.Cluster{Name: clusterName})
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

type provisionerInfo struct {
	Name        string                    `json:"name"`
	ClusterHelp provTypes.ClusterHelpInfo `json:"cluster_help"`
}

// title: list provisioners
// path: /provisioner
// method: GET
// produce: application/json
// responses:
//
//	200: Ok
//	204: No Content
//	401: Unauthorized
func provisionerList(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	allowed := permission.Check(ctx, t, permission.PermClusterRead)
	if !allowed {
		return permission.ErrUnauthorized
	}
	provs, err := provision.Registry()
	if err != nil {
		return err
	}
	info := make([]provisionerInfo, len(provs))
	for i, p := range provs {
		info[i].Name = p.GetName()
		if clusterProv, ok := p.(cluster.ClusteredProvisioner); ok {
			info[i].ClusterHelp = clusterProv.ClusterHelp()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(info)
}
