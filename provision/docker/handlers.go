// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ajg/form"
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
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/healer"
	"gopkg.in/mgo.v2"
)

func init() {
	api.RegisterHandler("/docker/container/{id}/move", "POST", api.AuthorizationRequiredHandler(moveContainerHandler))
	api.RegisterHandler("/docker/containers/move", "POST", api.AuthorizationRequiredHandler(moveContainersHandler))
	api.RegisterHandler("/docker/containers/rebalance", "POST", api.AuthorizationRequiredHandler(rebalanceContainersHandler))
	api.RegisterHandler("/docker/healing", "GET", api.AuthorizationRequiredHandler(healingHistoryHandler))
	api.RegisterHandler("/docker/autoscale", "GET", api.AuthorizationRequiredHandler(autoScaleHistoryHandler))
	api.RegisterHandler("/docker/autoscale/config", "GET", api.AuthorizationRequiredHandler(autoScaleGetConfig))
	api.RegisterHandler("/docker/autoscale/run", "POST", api.AuthorizationRequiredHandler(autoScaleRunHandler))
	api.RegisterHandler("/docker/autoscale/rules", "GET", api.AuthorizationRequiredHandler(autoScaleListRules))
	api.RegisterHandler("/docker/autoscale/rules", "POST", api.AuthorizationRequiredHandler(autoScaleSetRule))
	api.RegisterHandler("/docker/autoscale/rules", "DELETE", api.AuthorizationRequiredHandler(autoScaleDeleteRule))
	api.RegisterHandler("/docker/autoscale/rules/{id}", "DELETE", api.AuthorizationRequiredHandler(autoScaleDeleteRule))
	api.RegisterHandler("/docker/bs/upgrade", "POST", api.AuthorizationRequiredHandler(bsUpgradeHandler))
	api.RegisterHandler("/docker/bs/env", "POST", api.AuthorizationRequiredHandler(bsEnvSetHandler))
	api.RegisterHandler("/docker/bs", "GET", api.AuthorizationRequiredHandler(bsConfigGetHandler))
	api.RegisterHandler("/docker/logs", "GET", api.AuthorizationRequiredHandler(logsConfigGetHandler))
	api.RegisterHandler("/docker/logs", "POST", api.AuthorizationRequiredHandler(logsConfigSetHandler))
}

// title: get autoscale config
// path: /docker/autoscale/config
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
func autoScaleGetConfig(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowedGetConfig := permission.Check(t, permission.PermNodeAutoscaleRead)
	if !allowedGetConfig {
		return permission.ErrUnauthorized
	}
	config := mainDockerProvisioner.initAutoScaleConfig()
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(config)
}

// title: autoscale rules list
// path: /docker/autoscale/rules
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   204: No content
//   401: Unauthorized
func autoScaleListRules(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowedListRule := permission.Check(t, permission.PermNodeAutoscaleRead)
	if !allowedListRule {
		return permission.ErrUnauthorized
	}
	rules, err := listAutoScaleRules()
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return json.NewEncoder(w).Encode(&rules)
}

// title: autoscale set rule
// path: /docker/autoscale/rules
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
func autoScaleSetRule(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowedSetRule := permission.Check(t, permission.PermNodeAutoscaleUpdate)
	if !allowedSetRule {
		return permission.ErrUnauthorized
	}
	err = r.ParseForm()
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var rule autoScaleRule
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&rule, r.Form)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var ctxs []permission.PermissionContext
	if rule.MetadataFilter != "" {
		ctxs = append(ctxs, permission.Context(permission.CtxPool, rule.MetadataFilter))
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: rule.MetadataFilter},
		Kind:       permission.PermNodeAutoscaleUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, ctxs...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return rule.update()
}

// title: delete autoscale rule
// path: /docker/autoscale/rules/{id}
// method: DELETE
// responses:
//   200: Ok
//   401: Unauthorized
//   404: Not found
func autoScaleDeleteRule(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	allowedDeleteRule := permission.Check(t, permission.PermNodeAutoscale)
	if !allowedDeleteRule {
		return permission.ErrUnauthorized
	}
	rulePool := r.URL.Query().Get(":id")
	var ctxs []permission.PermissionContext
	if rulePool != "" {
		ctxs = append(ctxs, permission.Context(permission.CtxPool, rulePool))
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: rulePool},
		Kind:       permission.PermNodeAutoscaleDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, ctxs...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = deleteAutoScaleRule(rulePool)
	if err == mgo.ErrNotFound {
		return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: "rule not found"}
	}
	return nil
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
	err = r.ParseForm()
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	params := map[string]string{}
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&params, r.Form)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	contId := r.URL.Query().Get(":id")
	to := params["to"]
	if to == "" {
		return errors.Errorf("Invalid params: id: %s - to: %s", contId, to)
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
	_, err = mainDockerProvisioner.moveContainer(contId, to, writer)
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
	err = r.ParseForm()
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	params := map[string]string{}
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&params, r.Form)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	from := params["from"]
	to := params["to"]
	if from == "" || to == "" {
		return errors.Errorf("Invalid params: from: %s - to: %s", from, to)
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
	err = mainDockerProvisioner.MoveContainers(from, to, writer)
	if err != nil {
		return errors.Wrap(err, "Error trying to move containers")
	}
	fmt.Fprintf(writer, "Containers moved successfully!\n")
	return nil
}

func moveContainersPermissionContexts(from, to string) ([]permission.PermissionContext, error) {
	originHost, err := mainDockerProvisioner.GetNodeByHost(from)
	if err != nil {
		return nil, &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	destinationHost, err := mainDockerProvisioner.GetNodeByHost(to)
	if err != nil {
		return nil, &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	var permContexts []permission.PermissionContext
	originPool, ok := originHost.Metadata["pool"]
	if ok {
		permContexts = append(permContexts, permission.Context(permission.CtxPool, originPool))
	}
	if pool, ok := destinationHost.Metadata["pool"]; ok && pool != originPool {
		permContexts = append(permContexts, permission.Context(permission.CtxPool, pool))
	}
	return permContexts, nil
}

type rebalanceOptions struct {
	Dry            bool
	MetadataFilter map[string]string
	AppFilter      []string
}

// title: rebalance containers
// path: /docker/containers/rebalance
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Ok
//   204: No content
//   400: Invalid data
//   401: Unauthorized
func rebalanceContainersHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	var params rebalanceOptions
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&params, r.Form)
	if err != nil {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	var permContexts []permission.PermissionContext
	pool, ok := params.MetadataFilter["pool"]
	if ok {
		permContexts = append(permContexts, permission.Context(permission.CtxPool, pool))
	}
	if !permission.Check(t, permission.PermNodeUpdateRebalance, permContexts...) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:      event.Target{Type: event.TargetTypePool, Value: pool},
		Kind:        permission.PermNodeUpdateRebalance,
		Owner:       t,
		CustomData:  event.FormToCustomData(r.Form),
		DisableLock: true,
		Allowed:     event.Allowed(permission.PermPoolReadEvents, permContexts...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	_, err = mainDockerProvisioner.rebalanceContainersByFilter(writer, params.AppFilter, params.MetadataFilter, params.Dry)
	if err != nil {
		return errors.Wrap(err, "Error trying to rebalance containers")
	}
	fmt.Fprintf(writer, "Containers successfully rebalanced!\n")
	return nil
}

// title: list healing history
// path: /docker/healing
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   204: No content
//   400: Invalid data
//   401: Unauthorized
func healingHistoryHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermHealingRead) {
		return permission.ErrUnauthorized
	}
	filter := r.URL.Query().Get("filter")
	if filter != "" && filter != "node" && filter != "container" {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "invalid filter, possible values are 'node' or 'container'",
		}
	}
	history, err := healer.ListHealingHistory(filter)
	if err != nil {
		return err
	}
	if len(history) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(history)
}

// title: list autoscale history
// path: /docker/healing
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   204: No content
//   401: Unauthorized
func autoScaleHistoryHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermNodeAutoscale) {
		return permission.ErrUnauthorized
	}
	skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	history, err := listAutoScaleEvents(skip, limit)
	if err != nil {
		return err
	}
	if len(history) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(&history)
}

// title: autoscale run
// path: /docker/autoscale/run
// method: POST
// produce: application/x-json-stream
// responses:
//   200: Ok
//   401: Unauthorized
func autoScaleRunHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	if !permission.Check(t, permission.PermNodeAutoscaleUpdateRun) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:      event.Target{Type: event.TargetTypePool},
		Kind:        permission.PermNodeAutoscaleUpdateRun,
		Owner:       t,
		CustomData:  event.FormToCustomData(r.Form),
		DisableLock: true,
		Allowed:     event.Allowed(permission.PermPoolReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	w.WriteHeader(http.StatusOK)
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{
		Encoder: json.NewEncoder(keepAliveWriter),
	}
	autoScaleConfig := mainDockerProvisioner.initAutoScaleConfig()
	autoScaleConfig.writer = writer
	return autoScaleConfig.runOnce()
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
	err = r.ParseForm()
	if err != nil {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("unable to parse form values: %s", err),
		}
	}
	pool := r.FormValue("pool")
	restart, _ := strconv.ParseBool(r.FormValue("restart"))
	delete(r.Form, "pool")
	delete(r.Form, "restart")
	var conf container.DockerLogConfig
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&conf, r.Form)
	if err != nil {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("unable to parse fields in docker log config: %s", err),
		}
	}
	var ctxs []permission.PermissionContext
	if pool != "" {
		ctxs = append(ctxs, permission.Context(permission.CtxPool, pool))
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
	fmt.Fprintln(writer, "Log config successfully updated.")
	if restart {
		filter := &app.Filter{}
		if pool != "" {
			filter.Pools = []string{pool}
		}
		return tryRestartAppsByFilter(filter, writer)
	}
	return nil
}

func tryRestartAppsByFilter(filter *app.Filter, writer io.Writer) error {
	apps, err := app.List(filter)
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
			err := a.Restart("", writer)
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
