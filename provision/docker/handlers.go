// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	_ "github.com/tsuru/tsuru/iaas/cloudstack"
	_ "github.com/tsuru/tsuru/iaas/digitalocean"
	_ "github.com/tsuru/tsuru/iaas/dockermachine"
	_ "github.com/tsuru/tsuru/iaas/ec2"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/dockercommon"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

func init() {
	api.RegisterHandler("/docker/container/{id}/move", "POST", api.AuthorizationRequiredHandler(moveContainerHandler))
	api.RegisterHandler("/docker/containers/move", "POST", api.AuthorizationRequiredHandler(moveContainersHandler))
	api.RegisterHandler("/docker/bs/upgrade", "POST", api.AuthorizationRequiredHandler(bsUpgradeHandler))
	api.RegisterHandler("/docker/bs/env", "POST", api.AuthorizationRequiredHandler(bsEnvSetHandler))
	api.RegisterHandler("/docker/bs", "GET", api.AuthorizationRequiredHandler(bsConfigGetHandler))
	api.RegisterHandler("/docker/logs", "GET", api.AuthorizationRequiredHandler(logsConfigGetHandler))
	api.RegisterHandler("/docker/logs", "POST", api.AuthorizationRequiredHandler(logsConfigSetHandler))
}

// title: move container
// path: /docker/container/{id}/move
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Not found
func moveContainerHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	params := map[string]string{}
	err = api.ParseInput(r, &params)
	if err != nil {
		return err
	}
	contId := r.URL.Query().Get(":id")
	to := params["to"]
	if to == "" {
		return &tsuruErrors.ValidationError{Message: fmt.Sprintf("Invalid params: id: %q - to: %q", contId, to)}
	}
	cont, err := mainDockerProvisioner.GetContainer(contId)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	permContexts, err := moveContainersPermissionContexts(cont.HostAddr, to)
	if err != nil {
		return err
	}
	if !permission.Check(t, permission.PermNodeUpdateMoveContainer, permContexts...) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeContainer, Value: contId},
		Kind:       permission.PermNodeUpdateMoveContainer,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, permContexts...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt.SetLogWriter(writer)
	_, err = mainDockerProvisioner.moveContainer(contId, to, evt)
	if err != nil {
		return errors.Wrap(err, "Error trying to move container")
	}
	fmt.Fprintf(writer, "Containers moved successfully!\n")
	return nil
}

// title: move containers
// path: /docker/containers/move
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Not found
func moveContainersHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	params := map[string]string{}
	err = api.ParseInput(r, &params)
	if err != nil {
		return err
	}
	from := params["from"]
	to := params["to"]
	if from == "" || to == "" {
		return &tsuruErrors.ValidationError{Message: fmt.Sprintf("Invalid params: from: %q - to: %q", from, to)}
	}
	permContexts, err := moveContainersPermissionContexts(from, to)
	if err != nil {
		return err
	}
	if !permission.Check(t, permission.PermNodeUpdateMoveContainers, permContexts...) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeNode, Value: from},
		Kind:       permission.PermNodeUpdateMoveContainers,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, permContexts...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt.SetLogWriter(writer)
	err = mainDockerProvisioner.MoveContainers(from, to, evt)
	if err != nil {
		return errors.Wrap(err, "Error trying to move containers")
	}
	fmt.Fprintf(evt, "Containers moved successfully!\n")
	return nil
}

func moveContainersPermissionContexts(from, to string) ([]permTypes.PermissionContext, error) {
	originHost, err := dockercommon.GetNodeByHost(mainDockerProvisioner.Cluster(), from)
	if err != nil {
		return nil, &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	destinationHost, err := dockercommon.GetNodeByHost(mainDockerProvisioner.Cluster(), to)
	if err != nil {
		return nil, &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	var permContexts []permTypes.PermissionContext
	originPool, ok := originHost.Metadata[provision.PoolMetadataName]
	if ok {
		permContexts = append(permContexts, permission.Context(permTypes.CtxPool, originPool))
	}
	if pool, ok := destinationHost.Metadata[provision.PoolMetadataName]; ok && pool != originPool {
		permContexts = append(permContexts, permission.Context(permTypes.CtxPool, pool))
	}
	return permContexts, nil
}

func bsEnvSetHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return errors.New("this route is deprecated, please use POST /docker/nodecontainer/{name} (node-container-update command)")
}

func bsConfigGetHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return errors.New("this route is deprecated, please use GET /docker/nodecontainer/{name} (node-container-info command)")
}

func bsUpgradeHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return errors.New("this route is deprecated, please use POST /docker/nodecontainer/{name}/upgrade (node-container-upgrade command)")
}

// title: logs config
// path: /docker/logs
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
func logsConfigGetHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	pools, err := permission.ListContextValues(t, permission.PermPoolUpdateLogs, true)
	if err != nil {
		return err
	}
	configEntries, err := container.LogLoadAll()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	if len(pools) == 0 {
		return json.NewEncoder(w).Encode(configEntries)
	}
	newMap := map[string]container.DockerLogConfig{}
	for _, p := range pools {
		if entry, ok := configEntries[p]; ok {
			newMap[p] = entry
		}
	}
	return json.NewEncoder(w).Encode(newMap)
}

// title: logs config set
// path: /docker/logs
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
func logsConfigSetHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	pool := api.InputValue(r, "pool")
	restart, _ := strconv.ParseBool(api.InputValue(r, "restart"))
	var conf container.DockerLogConfig
	err = api.ParseInput(r, &conf)
	if err != nil {
		return err
	}
	var ctxs []permTypes.PermissionContext
	if pool != "" {
		ctxs = append(ctxs, permission.Context(permTypes.CtxPool, pool))
	}
	hasPermission := permission.Check(t, permission.PermPoolUpdateLogs, ctxs...)
	if !hasPermission {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:      event.Target{Type: event.TargetTypePool, Value: pool},
		Kind:        permission.PermPoolUpdateLogs,
		Owner:       t,
		CustomData:  event.FormToCustomData(r.Form),
		DisableLock: true,
		Allowed:     event.Allowed(permission.PermPoolReadEvents, ctxs...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = conf.Save(pool)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt.SetLogWriter(writer)
	fmt.Fprintln(evt, "Log config successfully updated.")
	if restart {
		filter := &app.Filter{}
		if pool != "" {
			filter.Pools = []string{pool}
		}
		return tryRestartAppsByFilter(filter, evt)
	}
	return nil
}

func tryRestartAppsByFilter(filter *app.Filter, writer io.Writer) error {
	ctx := context.TODO()
	apps, err := app.List(ctx, filter)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		return nil
	}
	appNames := make([]string, len(apps))
	for i, a := range apps {
		appNames[i] = a.Name
	}
	sort.Strings(appNames)
	fmt.Fprintf(writer, "Restarting %d applications: [%s]\n", len(apps), strings.Join(appNames, ", "))
	wg := sync.WaitGroup{}
	for i := range apps {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := apps[i]
			err := a.Restart(ctx, "", "", writer)
			if err != nil {
				fmt.Fprintf(writer, "Error: unable to restart %s: %s\n", a.Name, err.Error())
			} else {
				fmt.Fprintf(writer, "App %s successfully restarted\n", a.Name)
			}
		}(i)
	}
	wg.Wait()
	return nil
}
