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
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/rec"
	"github.com/tsuru/tsuru/service"
)

func serviceValidate(s service.Service) error {
	if s.Name == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Service id is required"}
	}
	if s.Password == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Service password is requried"}
	}
	if endpoint, ok := s.Endpoint["production"]; !ok || endpoint == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Service production endpoint is required"}
	}
	return nil
}

// title: service list
// path: /services
// method: GET
// produce: application/json
// responses:
//   200: List services
//   204: No content
//   401: Unauthorized
func serviceList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	rec.Log(t.GetUserName(), "list-services")
	teams := []string{}
	serviceNames := []string{}
	contexts := permission.ContextsForPermission(t, permission.PermServiceRead)
	for _, c := range contexts {
		if c.CtxType == permission.CtxGlobal {
			teams = nil
			serviceNames = nil
			break
		}
		switch c.CtxType {
		case permission.CtxService:
			serviceNames = append(serviceNames, c.Value)
		case permission.CtxTeam:
			teams = append(teams, c.Value)
		}
	}
	services, err := service.GetServicesByOwnerTeamsAndServices(teams, serviceNames)
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
			}
		}
	}
	if len(results) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	b, err := json.Marshal(results)
	if err != nil {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	n, err := w.Write(b)
	if n != len(b) {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: "Failed to write response body"}
	}
	w.Header().Set("Content-Type", "application/json")
	return err
}

// title: service create
// path: /services
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   201: Service created
//   400: Invalid data
//   401: Unauthorized
//   409: Service already exists
func serviceCreate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	s := service.Service{
		Name:     r.FormValue("id"),
		Username: r.FormValue("username"),
		Endpoint: map[string]string{"production": r.FormValue("endpoint")},
		Password: r.FormValue("password"),
	}
	team := r.FormValue("team")
	if team == "" {
		var err error
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
	}
	s.OwnerTeams = []string{team}
	err := serviceValidate(s)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceCreate,
		permission.Context(permission.CtxTeam, s.OwnerTeams[0]),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(t.GetUserName(), "create-service", s.Name, s.Endpoint["production"])
	err = s.Create()
	if err != nil {
		httpError := http.StatusInternalServerError
		if err == service.ErrServiceAlreadyExists {
			httpError = http.StatusConflict
		}
		return &errors.HTTP{Code: httpError, Message: err.Error()}
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
//   200: Service updated
//   400: Invalid data
//   401: Unauthorized
//   403: Forbidden (team is not the owner)
//   404: Service not found
func serviceUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	d := service.Service{
		Username: r.FormValue("username"),
		Endpoint: map[string]string{"production": r.FormValue("endpoint")},
		Password: r.FormValue("password"),
		Name:     r.URL.Query().Get(":name"),
	}
	err := serviceValidate(d)
	if err != nil {
		return err
	}
	s, err := getService(d.Name)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceUpdate,
		append(permission.Contexts(permission.CtxTeam, s.OwnerTeams),
			permission.Context(permission.CtxService, s.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(t.GetUserName(), "update-service", d.Name, d.Endpoint["production"])
	s.Endpoint = d.Endpoint
	s.Password = d.Password
	s.Username = d.Username
	if err = s.Update(); err != nil {
		return err
	}
	return nil
}

// title: service update
// path: /services/{name}
// method: DELETE
// responses:
//   200: Service removed
//   401: Unauthorized
//   403: Forbidden (team is not the owner or service with instances)
//   404: Service not found
func serviceDelete(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	s, err := getService(r.URL.Query().Get(":name"))
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceDelete,
		append(permission.Contexts(permission.CtxTeam, s.OwnerTeams),
			permission.Context(permission.CtxService, s.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(t.GetUserName(), "delete-service", r.URL.Query().Get(":name"))
	instances, err := service.GetServiceInstancesByServices([]service.Service{s})
	if err != nil {
		return err
	}
	if len(instances) > 0 {
		msg := "This service cannot be removed because it has instances.\n"
		msg += "Please remove these instances before removing the service."
		return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	err = s.Delete()
	if err != nil {
		return err
	}
	return nil
}

func serviceProxy(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	serviceName := r.URL.Query().Get(":service")
	s, err := getService(serviceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceUpdateProxy,
		append(permission.Contexts(permission.CtxTeam, s.OwnerTeams),
			permission.Context(permission.CtxService, s.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	path := r.URL.Query().Get("callback")
	return service.Proxy(&s, path, w, r)
}

// title: grant access to a service
// path: /services/{service}/team/{team}
// method: PUT
// responses:
//   200: Service updated
//   400: Team not found
//   401: Unauthorized
//   404: Service not found
//   409: Team already has access to this service
func grantServiceAccess(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	serviceName := r.URL.Query().Get(":service")
	s, err := getService(serviceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceUpdateGrantAccess,
		append(permission.Contexts(permission.CtxTeam, s.OwnerTeams),
			permission.Context(permission.CtxService, s.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	teamName := r.URL.Query().Get(":team")
	team, err := auth.GetTeam(teamName)
	if err != nil {
		if err == auth.ErrTeamNotFound {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: "Team not found"}
		}
		return err
	}
	rec.Log(t.GetUserName(), "grant-service-access", "service="+serviceName, "team="+teamName)
	err = s.GrantAccess(team)
	if err != nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	return s.Update()
}

// title: revoke access to a service
// path: /services/{service}/team/{team}
// method: DELETE
// responses:
//   200: Access revoked
//   400: Team not found
//   401: Unauthorized
//   404: Service not found
//   409: Team does not has access to this service
func revokeServiceAccess(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	serviceName := r.URL.Query().Get(":service")
	s, err := getService(serviceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceUpdateRevokeAccess,
		append(permission.Contexts(permission.CtxTeam, s.OwnerTeams),
			permission.Context(permission.CtxService, s.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	teamName := r.URL.Query().Get(":team")
	team, err := auth.GetTeam(teamName)
	if err != nil {
		if err == auth.ErrTeamNotFound {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: "Team not found"}
		}
		return err
	}
	if len(s.Teams) < 2 {
		msg := "You can not revoke the access from this team, because it is the unique team with access to this service, and a service can not be orphaned"
		return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	rec.Log(t.GetUserName(), "revoke-service-access", "service="+serviceName, "team="+teamName)
	err = s.RevokeAccess(team)
	if err != nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	return s.Update()
}

// title: change service documentation
// path: /services/{name}/doc
// consume: application/x-www-form-urlencoded
// method: PUT
// responses:
//   200: Documentation updated
//   401: Unauthorized
//   403: Forbidden (team is not the owner or service with instances)
func serviceAddDoc(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	serviceName := r.URL.Query().Get(":name")
	s, err := getService(serviceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceUpdateDoc,
		append(permission.Contexts(permission.CtxTeam, s.OwnerTeams),
			permission.Context(permission.CtxService, s.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	s.Doc = r.FormValue("doc")
	rec.Log(t.GetUserName(), "service-add-doc", serviceName, s.Doc)
	return s.Update()
}

func getService(name string) (service.Service, error) {
	s := service.Service{Name: name}
	err := s.Get()
	if err != nil {
		return s, &errors.HTTP{Code: http.StatusNotFound, Message: "Service not found"}
	}
	return s, err
}
