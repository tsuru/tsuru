// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ajg/form"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/service"
	permTypes "github.com/tsuru/tsuru/types/permission"
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
//   201: Service created
//   400: Invalid data
//   401: Unauthorized
//   409: Service already exists
func createServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	serviceName := r.URL.Query().Get(":service")
	srv, err := getService(serviceName)
	if err != nil {
		return err
	}
	err = r.ParseForm()
	if err != nil {
		return err
	}
	instance := service.ServiceInstance{
		ServiceName: serviceName,
		// for compatibility
		PlanName:  r.FormValue("plan"),
		TeamOwner: r.FormValue("owner"),
	}
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&instance, r.Form)
	if err != nil {
		return err
	}
	instance.Tags = append(instance.Tags, r.Form["tag"]...) // for compatibility
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
	evt, err := event.New(&event.Opts{
		Target:     serviceInstanceTarget(serviceName, instance.Name),
		Kind:       permission.PermServiceInstanceCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed: event.Allowed(permission.PermServiceInstanceReadEvents,
			contextsForServiceInstance(&instance, srv.Name)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	requestID := requestIDHeader(r)
	err = service.CreateServiceInstance(instance, &srv, evt, requestID)
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
//   200: Service instance updated
//   400: Invalid data
//   401: Unauthorized
//   404: Service instance not found
func updateServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	err = r.ParseForm()
	if err != nil {
		return err
	}
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	updateData := struct {
		Description string
		Plan        string
		TeamOwner   string
		Tags        []string
	}{}
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	dec.DecodeValues(&updateData, r.Form)
	updateData.Tags = append(updateData.Tags, r.Form["tag"]...) // for compatibility
	srv, err := getService(serviceName)
	if err != nil {
		return err
	}
	si, err := getServiceInstanceOrError(serviceName, instanceName)
	if err != nil {
		return err
	}
	var wantedPerms []*permission.PermissionScheme
	if updateData.Description != "" {
		wantedPerms = append(wantedPerms, permission.PermServiceInstanceUpdateDescription)
		si.Description = updateData.Description
	}
	if updateData.TeamOwner != "" {
		wantedPerms = append(wantedPerms, permission.PermServiceInstanceUpdateTeamowner)
		si.TeamOwner = updateData.TeamOwner
	}
	if updateData.Tags != nil {
		wantedPerms = append(wantedPerms, permission.PermServiceInstanceUpdateTags)
		si.Tags = updateData.Tags
	}
	if updateData.Plan != "" {
		wantedPerms = append(wantedPerms, permission.PermServiceInstanceUpdatePlan)
		si.PlanName = updateData.Plan
	}
	if len(wantedPerms) == 0 {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Neither the description, team owner, tags or plan were set. You must define at least one.",
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
	evt, err := event.New(&event.Opts{
		Target:     serviceInstanceTarget(serviceName, instanceName),
		Kind:       permission.PermServiceInstanceUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
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
//   200: Service removed
//   400: Bad request
//   401: Unauthorized
//   404: Service instance not found
func removeServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	unbindAll := r.URL.Query().Get("unbindall")
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
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
		CustomData: event.FormToCustomData(r.Form),
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
		if len(serviceInstance.Apps) > 0 {
			for _, appName := range serviceInstance.Apps {
				_, app, instErr := getServiceInstance(serviceInstance.ServiceName, serviceInstance.Name, appName)
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
			serviceInstance, err = getServiceInstanceOrError(serviceName, instanceName)
			if err != nil {
				return err
			}
		}
	}
	err = service.DeleteInstance(serviceInstance, evt, requestID)
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

func readableInstances(t auth.Token, contexts []permTypes.PermissionContext, appName, serviceName string) ([]service.ServiceInstance, error) {
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

func filtersForServiceList(t auth.Token, contexts []permTypes.PermissionContext) ([]string, []string) {
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

func readableServices(t auth.Token, contexts []permTypes.PermissionContext) ([]service.Service, error) {
	teams, serviceNames := filtersForServiceList(t, contexts)
	return service.GetServicesByTeamsAndServices(teams, serviceNames)
}

// title: service instance list
// path: /services/instances
// method: GET
// produce: application/json
// responses:
//   200: List services instances
//   204: No content
//   401: Unauthorized
func serviceInstances(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get("app")
	contexts := permission.ContextsForPermission(t, permission.PermServiceInstanceRead)
	instances, err := readableInstances(t, contexts, appName, "")
	if err != nil {
		return err
	}
	contexts = permission.ContextsForPermission(t, permission.PermServiceRead)
	services, err := readableServices(t, contexts)
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
//   200: List services instances
//   401: Unauthorized
//   404: Service instance not found
func serviceInstanceStatus(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
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
	Teams           []string
	TeamOwner       string
	Description     string
	PlanName        string
	PlanDescription string
	CustomInfo      map[string]string
	Tags            []string
}

// title: service instance info
// path: /services/{service}/instances/{instance}
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
//   404: Service instance not found
func serviceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	svc, err := getService(serviceName)
	if err != nil {
		return err
	}
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
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
	plan, err := service.GetPlanByServiceAndPlanName(svc, serviceInstance.PlanName, requestID)
	if err != nil {
		return err
	}
	sInfo := serviceInstanceInfo{
		Apps:            serviceInstance.Apps,
		Teams:           serviceInstance.Teams,
		TeamOwner:       serviceInstance.TeamOwner,
		Description:     serviceInstance.Description,
		PlanName:        plan.Name,
		PlanDescription: plan.Description,
		CustomInfo:      info,
		Tags:            serviceInstance.Tags,
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(sInfo)
}

// title: service info
// path: /services/{name}
// method: GET
// produce: application/json
// responses:
//   200: OK
func serviceInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	serviceName := r.URL.Query().Get(":name")
	_, err := getService(serviceName)
	if err != nil {
		return err
	}
	contexts := permission.ContextsForPermission(t, permission.PermServiceInstanceRead)
	instances, err := readableInstances(t, contexts, "", serviceName)
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
//   200: OK
//   401: Unauthorized
//   404: Not found
func serviceDoc(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	serviceName := r.URL.Query().Get(":name")
	s, err := getService(serviceName)
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

func getServiceInstanceOrError(serviceName string, instanceName string) (*service.ServiceInstance, error) {
	serviceInstance, err := service.GetServiceInstance(serviceName, instanceName)
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
//   200: OK
//   401: Unauthorized
//   404: Service not found
func servicePlans(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	serviceName := r.URL.Query().Get(":name")
	s, err := getService(serviceName)
	if err != nil {
		return err
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
	plans, err := service.GetPlansByService(s, requestID)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(plans)
}

func parseFormPreserveBody(r *http.Request) {
	var buf bytes.Buffer
	var readCloser struct {
		io.Reader
		io.Closer
	}
	if r.Body != nil {
		readCloser.Reader = io.TeeReader(r.Body, &buf)
		readCloser.Closer = r.Body
		r.Body = &readCloser
	}
	r.ParseForm()
	if buf.Len() > 0 {
		readCloser.Reader = &buf
	}
}

// title: service instance proxy
// path: /services/{service}/proxy/{instance}
// method: "*"
// responses:
//   401: Unauthorized
//   404: Instance not found
func serviceInstanceProxy(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	parseFormPreserveBody(r)
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
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
			Target: serviceInstanceTarget(serviceName, instanceName),
			Kind:   permission.PermServiceInstanceUpdateProxy,
			Owner:  t,
			CustomData: append(event.FormToCustomData(r.Form), map[string]interface{}{
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
	return service.ProxyInstance(serviceInstance, path, evt, requestIDHeader(r), w, r)
}

// title: grant access to service instance
// path: /services/{service}/instances/permission/{instance}/{team}
// consume: application/x-www-form-urlencoded
// method: PUT
// responses:
//   200: Access granted
//   401: Unauthorized
//   404: Service instance not found
func serviceInstanceGrantTeam(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
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
		CustomData: event.FormToCustomData(r.Form),
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
//   200: Access revoked
//   401: Unauthorized
//   404: Service instance not found
func serviceInstanceRevokeTeam(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
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
		CustomData: event.FormToCustomData(r.Form),
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
