// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	stdContext "context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	permTypes "github.com/tsuru/tsuru/types/permission"
	tagTypes "github.com/tsuru/tsuru/types/tag"
)

func serviceInstanceTarget(name, instance string) event.Target {
	return event.Target{Type: event.TargetTypeServiceInstance, Value: serviceIntancePermName(name, instance)}
}

func serviceIntancePermName(serviceName, instanceName string) string {
	return fmt.Sprintf("%s/%s", serviceName, instanceName)
}

// title: service instance create
// path: /services/{service}/instances
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//
//	201: Service created
//	400: Invalid data
//	401: Unauthorized
//	409: Service already exists
func createServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":service")
	srv, err := getService(ctx, serviceName)
	if err != nil {
		return err
	}
	instance := service.ServiceInstance{
		ServiceName: serviceName,
		// for compatibility
		PlanName:  InputValue(r, "plan"),
		TeamOwner: InputValue(r, "owner"),
	}
	err = ParseInput(r, &instance)
	if err != nil {
		return err
	}
	tags, _ := InputValues(r, "tag")
	instance.Tags = append(instance.Tags, tags...) // for compatibility
	var teamOwner string
	if instance.TeamOwner == "" {
		teamOwner, err = permission.TeamForPermission(t, permission.PermServiceInstanceCreate)
		if err != nil {
			return err
		}
		instance.TeamOwner = teamOwner
	}
	allowed := permission.Check(t, permission.PermServiceInstanceCreate,
		permission.Context(permTypes.CtxTeam, instance.TeamOwner),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	if srv.IsRestricted {
		allowed := permission.Check(t, permission.PermServiceRead,
			contextsForService(&srv)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}

	tagResponse, err := servicemanager.Tag.Validate(ctx, &tagTypes.TagValidationRequest{
		Operation: tagTypes.OperationKind_OPERATION_KIND_CREATE,
		Tags:      instance.Tags,
	})
	if err != nil {
		return err
	}
	if !tagResponse.Valid {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: tagResponse.Error}
	}

	evt, err := event.New(&event.Opts{
		Target:     serviceInstanceTarget(serviceName, instance.Name),
		Kind:       permission.PermServiceInstanceCreate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed: event.Allowed(permission.PermServiceInstanceReadEvents,
			contextsForServiceInstance(&instance, srv.Name)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	requestID := requestIDHeader(r)
	err = service.CreateServiceInstance(ctx, instance, &srv, evt, requestID)
	if err == service.ErrMultiClusterViolatingConstraint {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("Service %q is not available in pool %q", srv.Name, instance.Pool),
		}
	}
	if err == service.ErrInstanceNameAlreadyExists {
		return &tsuruErrors.HTTP{
			Code:    http.StatusConflict,
			Message: err.Error(),
		}
	}
	if err == service.ErrInvalidInstanceName {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if err == nil {
		w.WriteHeader(http.StatusCreated)
	}
	return err
}

// title: service instance update
// path: /services/{service}/instances/{instance}
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//
//	200: Service instance updated
//	400: Invalid data
//	401: Unauthorized
//	404: Service instance not found
func updateServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	updateData := struct {
		Description string
		Plan        string
		TeamOwner   string
		Tags        []string
		Parameters  map[string]interface{}
	}{}
	err = ParseInput(r, &updateData)
	if err != nil {
		return err
	}
	tags, _ := InputValues(r, "tag")
	updateData.Tags = append(updateData.Tags, tags...) // for compatibility
	srv, err := getService(ctx, serviceName)
	if err != nil {
		return err
	}
	si, err := getServiceInstanceOrError(ctx, serviceName, instanceName)
	if err != nil {
		return err
	}
	var wantedPerms []*permission.PermissionScheme
	if si.Description != updateData.Description {
		wantedPerms = append(wantedPerms, permission.PermServiceInstanceUpdateDescription)
		si.Description = updateData.Description
	}
	if si.TeamOwner != updateData.TeamOwner {
		wantedPerms = append(wantedPerms, permission.PermServiceInstanceUpdateTeamowner)
		si.TeamOwner = updateData.TeamOwner
	}
	if si.PlanName != updateData.Plan {
		wantedPerms = append(wantedPerms, permission.PermServiceInstanceUpdatePlan)
		si.PlanName = updateData.Plan
	}
	if !reflect.DeepEqual(si.Tags, updateData.Tags) {
		wantedPerms = append(wantedPerms, permission.PermServiceInstanceUpdateTags)
		si.Tags = updateData.Tags
	}
	if !reflect.DeepEqual(si.Parameters, updateData.Parameters) {
		wantedPerms = append(wantedPerms, permission.PermServiceInstanceUpdateParameters)
		si.Parameters = updateData.Parameters
	}
	if len(wantedPerms) == 0 {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Neither description, team owner, tags, plan nor plan parameters were set. You must define at least one.",
		}
	}
	for _, perm := range wantedPerms {
		allowed := permission.Check(t, perm,
			contextsForServiceInstance(si, serviceName)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	tagResponse, err := servicemanager.Tag.Validate(ctx, &tagTypes.TagValidationRequest{
		Operation: tagTypes.OperationKind_OPERATION_KIND_UPDATE,
		Tags:      si.Tags,
	})
	if err != nil {
		return err
	}
	if !tagResponse.Valid {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: tagResponse.Error}
	}
	evt, err := event.New(&event.Opts{
		Target:     serviceInstanceTarget(serviceName, instanceName),
		Kind:       permission.PermServiceInstanceUpdate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed: event.Allowed(permission.PermServiceInstanceReadEvents,
			contextsForServiceInstance(si, serviceName)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	requestID := requestIDHeader(r)
	return si.Update(srv, *si, evt, requestID)
}

// title: remove service instance
// path: /services/{name}/instances/{instance}
// method: DELETE
// produce: application/x-json-stream
// responses:
//
//	200: Service removed
//	400: Bad request
//	401: Unauthorized
//	404: Service instance not found
func removeServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	ignoreErrors := r.URL.Query().Get("ignoreerrors")
	unbindAll := r.URL.Query().Get("unbindall")
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	serviceInstance, err := getServiceInstanceOrError(ctx, serviceName, instanceName)
	if err != nil {
		return err
	}
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	w.Header().Set("Content-Type", "application/x-json-stream")
	allowed := permission.Check(t, permission.PermServiceInstanceDelete,
		contextsForServiceInstance(serviceInstance, serviceName)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     serviceInstanceTarget(serviceName, instanceName),
		Kind:       permission.PermServiceInstanceDelete,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed: event.Allowed(permission.PermServiceInstanceReadEvents,
			contextsForServiceInstance(serviceInstance, serviceName)...),
	})
	if err != nil {
		return err
	}
	evt.SetLogWriter(writer)
	defer func() { evt.Done(err) }()
	requestID := requestIDHeader(r)
	unbindAllBool, _ := strconv.ParseBool(unbindAll)
	if unbindAllBool {
		for _, appName := range serviceInstance.Apps {
			_, app, instErr := getServiceInstance(ctx, serviceInstance.ServiceName, serviceInstance.Name, appName)
			if instErr != nil {
				return instErr
			}
			fmt.Fprintf(evt, "Unbind app %q ...\n", app.GetName())
			instErr = serviceInstance.UnbindApp(service.UnbindAppArgs{
				App:         app,
				Restart:     true,
				ForceRemove: false,
				Event:       evt,
				RequestID:   requestID,
			})
			if instErr != nil {
				return instErr
			}
			fmt.Fprintf(evt, "\nInstance %q is not bound to the app %q anymore.\n", serviceInstance.Name, app.GetName())
		}
		serviceInstance, err = getServiceInstanceOrError(ctx, serviceName, instanceName)
		if err != nil {
			return err
		}
	}
	ignoreErrorsBool, _ := strconv.ParseBool(ignoreErrors)
	serviceInstance.ForceRemove = ignoreErrorsBool
	err = service.DeleteInstance(ctx, serviceInstance, evt, requestID)
	if err != nil {
		if err == service.ErrServiceInstanceBound {
			return &tsuruErrors.HTTP{
				Message: errors.Wrapf(err, `Applications bound to the service "%s": "%s"`+"\n", instanceName, strings.Join(serviceInstance.Apps, ",")).Error(),
				Code:    http.StatusBadRequest,
			}
		}
		return err
	}
	evt.Write([]byte("service instance successfully removed\n"))
	return nil
}

func readableInstances(contexts []permTypes.PermissionContext, appName, serviceName string) ([]service.ServiceInstance, error) {
	teams := []string{}
	instanceNames := []string{}
	for _, c := range contexts {
		if c.CtxType == permTypes.CtxGlobal {
			teams = nil
			instanceNames = nil
			break
		}
		switch c.CtxType {
		case permTypes.CtxServiceInstance:
			parts := strings.SplitN(c.Value, "/", 2)
			if len(parts) == 2 && (serviceName == "" || parts[0] == serviceName) {
				instanceNames = append(instanceNames, parts[1])
			}
		case permTypes.CtxTeam:
			teams = append(teams, c.Value)
		}
	}
	return service.GetServicesInstancesByTeamsAndNames(teams, instanceNames, appName, serviceName)
}

func filtersForServiceList(contexts []permTypes.PermissionContext) ([]string, []string) {
	teams := []string{}
	serviceNames := []string{}
	for _, c := range contexts {
		if c.CtxType == permTypes.CtxGlobal {
			teams = nil
			serviceNames = nil
			break
		}
		switch c.CtxType {
		case permTypes.CtxService:
			serviceNames = append(serviceNames, c.Value)
		case permTypes.CtxTeam:
			teams = append(teams, c.Value)
		}
	}
	return teams, serviceNames
}

func readableServices(ctx stdContext.Context, contexts []permTypes.PermissionContext) ([]service.Service, error) {
	teams, serviceNames := filtersForServiceList(contexts)
	return service.GetServicesByTeamsAndServices(ctx, teams, serviceNames)
}

// title: service instance list
// path: /services/instances
// method: GET
// produce: application/json
// responses:
//
//	200: List services instances
//	204: No content
//	401: Unauthorized
func serviceInstances(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	appName := r.URL.Query().Get("app")
	contexts := permission.ContextsForPermission(t, permission.PermServiceInstanceRead)
	instances, err := readableInstances(contexts, appName, "")
	if err != nil {
		return err
	}
	contexts = permission.ContextsForPermission(t, permission.PermServiceRead)
	services, err := readableServices(ctx, contexts)
	if err != nil {
		return err
	}
	servicesMap := map[string]*service.ServiceModel{}
	for _, s := range services {
		if _, in := servicesMap[s.Name]; !in {
			servicesMap[s.Name] = &service.ServiceModel{
				Service:   s.Name,
				Instances: []string{},
			}
		}
	}
	for _, instance := range instances {
		entry := servicesMap[instance.ServiceName]
		if entry == nil {
			continue
		}
		entry.Instances = append(entry.Instances, instance.Name)
		entry.Plans = append(entry.Plans, instance.PlanName)
		entry.ServiceInstances = append(entry.ServiceInstances, instance)
	}
	result := []service.ServiceModel{}
	for _, name := range sortedServiceNames(servicesMap) {
		entry := servicesMap[name]
		result = append(result, *entry)
	}
	if len(result) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(result)
}

// title: service instance status
// path: /services/{service}/instances/{instance}/status
// method: GET
// responses:
//
//	200: List services instances
//	401: Unauthorized
//	404: Service instance not found
func serviceInstanceStatus(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	serviceInstance, err := getServiceInstanceOrError(ctx, serviceName, instanceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceReadStatus,
		contextsForServiceInstance(serviceInstance, serviceName)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	var b string
	requestID := requestIDHeader(r)
	if b, err = serviceInstance.Status(requestID); err != nil {
		return errors.Wrap(err, "Could not retrieve status of service instance, error")
	}
	_, err = fmt.Fprintf(w, `Service instance "%s" is %s`, instanceName, b)
	return err
}

type serviceInstanceInfo struct {
	Apps            []string
	Jobs            []string
	Teams           []string
	TeamOwner       string
	Description     string
	PlanName        string
	PlanDescription string
	Pool            string
	CustomInfo      map[string]string
	Tags            []string
	Parameters      map[string]interface{}
}

// title: service instance info
// path: /services/{service}/instances/{instance}
// method: GET
// produce: application/json
// responses:
//
//	200: OK
//	401: Unauthorized
//	404: Service instance not found
func serviceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	svc, err := getService(ctx, serviceName)
	if err != nil {
		return err
	}
	serviceInstance, err := getServiceInstanceOrError(ctx, serviceName, instanceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceRead,
		contextsForServiceInstance(serviceInstance, serviceName)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	requestID := requestIDHeader(r)
	info, err := serviceInstance.Info(requestID)
	if err != nil {
		return err
	}
	plan, err := service.GetPlanByServiceAndPlanName(ctx, svc, serviceInstance.Pool, serviceInstance.PlanName, requestID)
	if err != nil {
		return err
	}
	sInfo := serviceInstanceInfo{
		Apps:            serviceInstance.Apps,
		Jobs:            serviceInstance.Jobs,
		Teams:           serviceInstance.Teams,
		TeamOwner:       serviceInstance.TeamOwner,
		Description:     serviceInstance.Description,
		Pool:            serviceInstance.Pool,
		PlanName:        plan.Name,
		PlanDescription: plan.Description,
		CustomInfo:      info,
		Tags:            serviceInstance.Tags,
		Parameters:      serviceInstance.Parameters,
	}
	if sInfo.PlanName == "" {
		sInfo.PlanName = serviceInstance.PlanName
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(sInfo)
}

// title: service info
// path: /services/{name}
// method: GET
// produce: application/json
// responses:
//
//	200: OK
func serviceInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":name")
	_, err := getService(ctx, serviceName)
	if err != nil {
		return err
	}
	contexts := permission.ContextsForPermission(t, permission.PermServiceInstanceRead)
	instances, err := readableInstances(contexts, "", serviceName)
	if err != nil {
		return err
	}
	var result []service.ServiceInstanceWithInfo
	for _, instance := range instances {
		infoData, err := instance.ToInfo()
		if err != nil {
			return err
		}
		result = append(result, infoData)
	}
	return json.NewEncoder(w).Encode(result)
}

// title: service doc
// path: /services/{name}/doc
// method: GET
// responses:
//
//	200: OK
//	401: Unauthorized
//	404: Not found
func serviceDoc(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":name")
	s, err := getService(ctx, serviceName)
	if err != nil {
		return err
	}
	if s.IsRestricted {
		allowed := permission.Check(t, permission.PermServiceReadDoc,
			contextsForService(&s)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	w.Write([]byte(s.Doc))
	return nil
}

func getServiceInstanceOrError(ctx stdContext.Context, serviceName string, instanceName string) (*service.ServiceInstance, error) {
	serviceInstance, err := service.GetServiceInstance(ctx, serviceName, instanceName)
	if err != nil {
		switch err {
		case service.ErrServiceInstanceNotFound:
			return nil, &tsuruErrors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		default:
			return nil, err
		}
	}
	return serviceInstance, nil
}

// title: service plans
// path: /services/{name}/plans
// method: GET
// produce: application/json
// responses:
//
//	200: OK
//	401: Unauthorized
//	404: Service not found
func servicePlans(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":name")
	pool := ""
	s, err := getService(ctx, serviceName)
	if err != nil {
		return err
	}

	if s.IsMultiCluster {
		pool = r.URL.Query().Get("pool")
	}

	if s.IsRestricted {
		allowed := permission.Check(t, permission.PermServiceReadPlans,
			contextsForService(&s)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	requestID := requestIDHeader(r)
	plans, err := service.GetPlansByService(ctx, s, pool, requestID)
	if err == service.ErrMissingPool {
		availablePools, poolErr := possiblePoolsForService(ctx, t)
		if poolErr != nil {
			return poolErr
		}
		return &tsuruErrors.ValidationError{
			Message: fmt.Sprintf("You must provide the pool name, available pools: %s", strings.Join(availablePools, ", ")),
		}
	}

	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(plans)
}

func possiblePoolsForService(ctx stdContext.Context, t auth.Token) ([]string, error) {
	global, teams := teamsForToken(t)
	var pools []pool.Pool
	var err error
	if global {
		pools, err = pool.ListAllPools(ctx)
	} else {
		pools, err = pool.ListPossiblePools(ctx, teams)
	}

	if err != nil {
		return nil, err
	}

	result := []string{}
	for _, pool := range pools {
		result = append(result, pool.Name)
	}
	sort.Strings(result)
	return result, nil
}

func teamsForToken(t auth.Token) (global bool, teams []string) {
	contexts := permission.ContextsForPermission(t, permission.PermServiceInstanceRead)
	teams = []string{}

	for _, c := range contexts {
		if c.CtxType == permTypes.CtxGlobal {
			return true, nil
		}

		if c.CtxType == permTypes.CtxTeam {
			teams = append(teams, c.Value)
		}
	}

	return false, teams
}

// title: service instance proxy
// path: /services/{service}/proxy/{instance}
// method: "*"
// responses:
//
//	401: Unauthorized
//	404: Instance not found
func serviceInstanceProxy(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	serviceInstance, err := getServiceInstanceOrError(ctx, serviceName, instanceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateProxy,
		contextsForServiceInstance(serviceInstance, serviceName)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	path := r.URL.Query().Get("callback")
	var evt *event.Event
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		evt, err = event.New(&event.Opts{
			Target:     serviceInstanceTarget(serviceName, instanceName),
			Kind:       permission.PermServiceInstanceUpdateProxy,
			Owner:      t,
			RemoteAddr: r.RemoteAddr,
			CustomData: append(event.FormToCustomData(InputFields(r)), map[string]interface{}{
				"name":  "method",
				"value": r.Method,
			}),
			Allowed: event.Allowed(permission.PermServiceInstanceReadEvents,
				contextsForServiceInstance(serviceInstance, serviceName)...),
		})
		if err != nil {
			return err
		}
		defer func() { evt.Done(err) }()
	}
	return service.ProxyInstance(ctx, serviceInstance, path, evt, requestIDHeader(r), w, r)
}

// title: service instance proxy V2
// path: /services/{service}/resources/{instance}/{path:*}
// method: "*"
// responses:
//
//	401: Unauthorized
//	404: Instance not found
func serviceInstanceProxyV2(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	queryPath := r.URL.Query().Get(":path")

	serviceInstance, err := getServiceInstanceOrError(ctx, serviceName, instanceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateProxy,
		contextsForServiceInstance(serviceInstance, serviceName)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	path := "/resources/" + serviceInstance.GetIdentifier() + "/" + queryPath

	var evt *event.Event
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		evt, err = event.New(&event.Opts{
			Target:     serviceInstanceTarget(serviceName, instanceName),
			Kind:       permission.PermServiceInstanceUpdateProxy,
			Owner:      t,
			RemoteAddr: r.RemoteAddr,
			CustomData: append(event.FormToCustomData(InputFields(r)), map[string]interface{}{
				"name":  "method",
				"value": r.Method,
			}),
			Allowed: event.Allowed(permission.PermServiceInstanceReadEvents,
				contextsForServiceInstance(serviceInstance, serviceName)...),
		})
		if err != nil {
			return err
		}
		defer func() { evt.Done(err) }()
	}
	return service.ProxyInstance(ctx, serviceInstance, path, evt, requestIDHeader(r), w, r)
}

// title: grant access to service instance
// path: /services/{service}/instances/permission/{instance}/{team}
// consume: application/x-www-form-urlencoded
// method: PUT
// responses:
//
//	200: Access granted
//	401: Unauthorized
//	404: Service instance not found
func serviceInstanceGrantTeam(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	serviceInstance, err := getServiceInstanceOrError(ctx, serviceName, instanceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateGrant,
		contextsForServiceInstance(serviceInstance, serviceName)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     serviceInstanceTarget(serviceName, instanceName),
		Kind:       permission.PermServiceInstanceUpdateGrant,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed: event.Allowed(permission.PermServiceInstanceReadEvents,
			contextsForServiceInstance(serviceInstance, serviceName)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	teamName := r.URL.Query().Get(":team")
	return serviceInstance.Grant(teamName)
}

// title: revoke access to service instance
// path: /services/{service}/instances/permission/{instance}/{team}
// method: DELETE
// responses:
//
//	200: Access revoked
//	401: Unauthorized
//	404: Service instance not found
func serviceInstanceRevokeTeam(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	serviceInstance, err := getServiceInstanceOrError(ctx, serviceName, instanceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateRevoke,
		contextsForServiceInstance(serviceInstance, serviceName)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     serviceInstanceTarget(serviceName, instanceName),
		Kind:       permission.PermServiceInstanceUpdateRevoke,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed: event.Allowed(permission.PermServiceInstanceReadEvents,
			contextsForServiceInstance(serviceInstance, serviceName)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	teamName := r.URL.Query().Get(":team")
	return serviceInstance.Revoke(teamName)
}

func contextsForServiceInstance(si *service.ServiceInstance, serviceName string) []permTypes.PermissionContext {
	permissionValue := serviceIntancePermName(serviceName, si.Name)
	return append(permission.Contexts(permTypes.CtxTeam, si.Teams),
		permission.Context(permTypes.CtxServiceInstance, permissionValue),
	)
}

func contextsForService(s *service.Service) []permTypes.PermissionContext {
	return append(permission.Contexts(permTypes.CtxTeam, s.Teams),
		permission.Context(permTypes.CtxService, s.Name),
	)
}

func sortedServiceNames(services map[string]*service.ServiceModel) []string {
	serviceNames := make([]string, len(services))
	i := 0
	for s := range services {
		serviceNames[i] = s
		i++
	}
	sort.Strings(serviceNames)
	return serviceNames
}

func requestIDHeader(r *http.Request) string {
	requestIDHeader, _ := config.GetString("request-id-header")
	return context.GetRequestID(r, requestIDHeader)
}
