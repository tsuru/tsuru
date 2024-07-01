// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

func serviceTarget(name string) eventTypes.Target {
	return eventTypes.Target{Type: eventTypes.TargetTypeService, Value: name}
}

func provisionReadableServices(ctx context.Context, contexts []permTypes.PermissionContext) ([]service.Service, error) {
	teams, serviceNames := filtersForServiceList(contexts)
	return service.GetServicesByOwnerTeamsAndServices(ctx, teams, serviceNames)
}

// title: service list
// path: /services
// method: GET
// produce: application/json
// responses:
//
//	200: List services
//	204: No content
//	401: Unauthorized
func serviceList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	contexts := permission.ContextsForPermission(t, permission.PermServiceRead)
	services, err := provisionReadableServices(ctx, contexts)
	if err != nil {
		return err
	}
	sInstances, err := service.GetServiceInstancesByServices(services)
	if err != nil {
		return err
	}
	results := make([]service.ServiceModel, len(services))
	for i, s := range services {
		results[i].Service = s.Name
		for _, si := range sInstances {
			if si.ServiceName == s.Name {
				results[i].Instances = append(results[i].Instances, si.Name)
				results[i].ServiceInstances = append(results[i].ServiceInstances, si)
			}
		}
	}
	if len(results) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(results)
}

type serviceInput struct {
	Name      string            `json:"id" form:"id"`
	Username  string            `json:"username" form:"username"`
	Password  string            `json:"password" form:"password"`
	Endpoints map[string]string `json:"endpoints" form:"endpoints"`
	Endpoint  string            `json:"endpoint" form:"endpoint"`
}

func parseService(r *http.Request) (service.Service, error) {
	var s service.Service

	var inputSvc serviceInput
	err := ParseInput(r, &inputSvc)
	if err != nil {
		return s, &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	s.Name = inputSvc.Name
	s.Username = inputSvc.Username
	s.Password = inputSvc.Password
	if len(inputSvc.Endpoints) != 0 {
		s.Endpoint = inputSvc.Endpoints
	} else if inputSvc.Endpoint != "" {
		s.Endpoint = map[string]string{"production": inputSvc.Endpoint}
	}

	multiCluster, err := strconv.ParseBool(InputValue(r, "multi-cluster"))
	if err == nil {
		s.IsMultiCluster = multiCluster
	}
	team := InputValue(r, "team")
	if team != "" {
		s.OwnerTeams = []string{team}
	}
	return s, nil
}

// title: service create
// path: /services
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//
//	201: Service created
//	400: Invalid data
//	401: Unauthorized
//	409: Service already exists
func serviceCreate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	s, err := parseService(r)
	if err != nil {
		return err
	}

	if len(s.OwnerTeams) == 0 {
		var team string
		team, err = permission.TeamForPermission(t, permission.PermServiceCreate)
		if err == permission.ErrTooManyTeams {
			return &errors.HTTP{
				Code:    http.StatusBadRequest,
				Message: "You must provide a team responsible for this service in the manifest file.",
			}
		}
		if err != nil {
			return err
		}
		s.OwnerTeams = []string{team}
	}
	allowed := permission.Check(t, permission.PermServiceCreate,
		permission.Context(permTypes.CtxTeam, s.OwnerTeams[0]),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	delete(r.Form, "password")
	evt, err := event.New(ctx, &event.Opts{
		Target:     serviceTarget(s.Name),
		Kind:       permission.PermServiceCreate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermServiceReadEvents, contextsForServiceProvision(&s)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	err = service.Create(ctx, s)
	if err != nil {
		if err == service.ErrServiceAlreadyExists {
			return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
		}
		return err
	}
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, "success")
	return nil
}

// title: service update
// path: /services/{name}
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//
//	200: Service updated
//	400: Invalid data
//	401: Unauthorized
//	403: Forbidden (team is not the owner)
//	404: Service not found
func serviceUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	d, err := parseService(r)
	if err != nil {
		return err
	}
	d.Name = r.URL.Query().Get(":name")

	ctx := r.Context()
	s, err := getService(ctx, d.Name)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceUpdate,
		contextsForServiceProvision(&s)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	delete(r.Form, "password")
	evt, err := event.New(ctx, &event.Opts{
		Target:     serviceTarget(s.Name),
		Kind:       permission.PermServiceUpdate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermServiceReadEvents, contextsForServiceProvision(&s)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	s.Endpoint = d.Endpoint
	s.Password = d.Password
	s.Username = d.Username
	if len(d.OwnerTeams) != 0 {
		s.OwnerTeams = d.OwnerTeams
	}
	return service.Update(ctx, s)
}

// title: service delete
// path: /services/{name}
// method: DELETE
// responses:
//
//	200: Service removed
//	401: Unauthorized
//	403: Forbidden (team is not the owner or service with instances)
//	404: Service not found
func serviceDelete(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	s, err := getService(ctx, r.URL.Query().Get(":name"))
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceDelete,
		contextsForServiceProvision(&s)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     serviceTarget(s.Name),
		Kind:       permission.PermServiceDelete,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermServiceReadEvents, contextsForServiceProvision(&s)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	instances, err := service.GetServiceInstancesByServices([]service.Service{s})
	if err != nil {
		return err
	}
	if len(instances) > 0 {
		msg := "This service cannot be removed because it has instances.\n"
		msg += "Please remove these instances before removing the service."
		return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	return service.Delete(s)
}

// title: service proxy
// path: /services/proxy/service/{service}
// method: "*"
// responses:
//
//	401: Unauthorized
//	404: Service not found
func serviceProxy(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":service")
	s, err := getService(ctx, serviceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceUpdateProxy,
		contextsForServiceProvision(&s)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	var evt *event.Event
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		evt, err = event.New(ctx, &event.Opts{
			Target:     serviceTarget(s.Name),
			Kind:       permission.PermServiceUpdateProxy,
			Owner:      t,
			RemoteAddr: r.RemoteAddr,
			CustomData: append(event.FormToCustomData(InputFields(r)), map[string]interface{}{
				"name":  "method",
				"value": r.Method,
			}),
			Allowed: event.Allowed(permission.PermServiceReadEvents, contextsForServiceProvision(&s)...),
		})
		if err != nil {
			return err
		}
		defer func() { evt.Done(ctx, err) }()
	}
	path := r.URL.Query().Get("callback")
	return service.Proxy(ctx, &s, path, evt, requestIDHeader(r), w, r)
}

// title: service proxy for authenticated resources, that does not have permission to check
// path: /services/{service}/authenticated-resources/{path:.*}
// method: "*"
// responses:
//
//	401: Unauthorized
//	404: Service not found
func serviceAuthenticatedResourcesProxy(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":service")
	queryPath := r.URL.Query().Get(":path")
	s, err := getService(ctx, serviceName)
	if err != nil {
		return err
	}
	var evt *event.Event
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		evt, err = event.New(ctx, &event.Opts{
			Target:     serviceTarget(s.Name),
			Kind:       permission.PermServiceUpdateProxy,
			Owner:      t,
			RemoteAddr: r.RemoteAddr,
			CustomData: append(event.FormToCustomData(InputFields(r)), map[string]interface{}{
				"name":  "method",
				"value": r.Method,
			}),
			Allowed: event.Allowed(permission.PermServiceReadEvents, contextsForServiceProvision(&s)...),
		})
		if err != nil {
			return err
		}
		defer func() { evt.Done(ctx, err) }()
	}

	path := "/authenticated-resources/" + queryPath

	return service.Proxy(ctx, &s, path, evt, requestIDHeader(r), w, r)
}

// title: grant access to a service
// path: /services/{service}/team/{team}
// method: PUT
// responses:
//
//	200: Service updated
//	400: Team not found
//	401: Unauthorized
//	404: Service not found
//	409: Team already has access to this service
func grantServiceAccess(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":service")
	s, err := getService(ctx, serviceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceUpdateGrantAccess,
		contextsForServiceProvision(&s)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	teamName := r.URL.Query().Get(":team")
	team, err := servicemanager.Team.FindByName(ctx, teamName)
	if err != nil {
		if err == authTypes.ErrTeamNotFound {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: "Team not found"}
		}
		return err
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     serviceTarget(s.Name),
		Kind:       permission.PermServiceUpdateGrantAccess,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermServiceReadEvents, contextsForServiceProvision(&s)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	err = s.GrantAccess(team)
	if err != nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	return service.Update(ctx, s)
}

// title: revoke access to a service
// path: /services/{service}/team/{team}
// method: DELETE
// responses:
//
//	200: Access revoked
//	400: Team not found
//	401: Unauthorized
//	404: Service not found
//	409: Team does not has access to this service
func revokeServiceAccess(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":service")
	s, err := getService(ctx, serviceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceUpdateRevokeAccess,
		contextsForServiceProvision(&s)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	teamName := r.URL.Query().Get(":team")
	team, err := servicemanager.Team.FindByName(ctx, teamName)
	if err != nil {
		if err == authTypes.ErrTeamNotFound {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: "Team not found"}
		}
		return err
	}
	if len(s.Teams) < 2 {
		msg := "You can not revoke the access from this team, because it is the unique team with access to this service, and a service can not be orphaned"
		return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     serviceTarget(s.Name),
		Kind:       permission.PermServiceUpdateRevokeAccess,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermServiceReadEvents, contextsForServiceProvision(&s)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	err = s.RevokeAccess(team)
	if err != nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	return service.Update(ctx, s)
}

// title: change service documentation
// path: /services/{name}/doc
// consume: application/x-www-form-urlencoded
// method: PUT
// responses:
//
//	200: Documentation updated
//	401: Unauthorized
//	403: Forbidden (team is not the owner or service with instances)
func serviceAddDoc(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	serviceName := r.URL.Query().Get(":name")
	s, err := getService(ctx, serviceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceUpdateDoc,
		contextsForServiceProvision(&s)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	s.Doc = InputValue(r, "doc")
	evt, err := event.New(ctx, &event.Opts{
		Target:     serviceTarget(s.Name),
		Kind:       permission.PermServiceUpdateDoc,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermServiceReadEvents, contextsForServiceProvision(&s)...),
	})
	if err != nil {
		return err
	}

	defer func() { evt.Done(ctx, err) }()
	return service.Update(ctx, s)
}

func getService(ctx context.Context, name string) (service.Service, error) {
	s, err := service.Get(ctx, name)
	if err == service.ErrServiceNotFound {
		return s, &errors.HTTP{Code: http.StatusNotFound, Message: "Service not found"}
	}
	return s, err
}

func contextsForServiceProvision(s *service.Service) []permTypes.PermissionContext {
	return append(permission.Contexts(permTypes.CtxTeam, s.OwnerTeams),
		permission.Context(permTypes.CtxService, s.Name),
	)
}
