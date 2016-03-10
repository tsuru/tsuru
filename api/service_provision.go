// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/rec"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/yaml.v1"
)

type serviceYaml struct {
	Id       string
	Username string
	Password string
	Endpoint map[string]string
	Team     string
}

func (sy *serviceYaml) validate() error {
	if sy.Id == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "You must provide an id in the manifest file."}
	}
	if sy.Password == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "You must provide a password in the manifest file."}
	}
	if _, ok := sy.Endpoint["production"]; !ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "You must provide a production endpoint in the manifest file."}
	}
	return nil
}

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
	return err
}

func serviceCreate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var input serviceYaml
	err = yaml.Unmarshal(body, &input)
	if err != nil {
		return err
	}
	if input.Team == "" {
		input.Team, err = permission.TeamForPermission(t, permission.PermServiceCreate)
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
	err = input.validate()
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceCreate,
		permission.Context(permission.CtxTeam, input.Team),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(t.GetUserName(), "create-service", input.Id, input.Endpoint)
	s := service.Service{
		Name:       input.Id,
		Username:   input.Username,
		Endpoint:   input.Endpoint,
		Password:   input.Password,
		OwnerTeams: []string{input.Team},
	}
	err = s.Create()
	if err != nil {
		httpError := http.StatusInternalServerError
		if err == service.ErrServiceAlreadyExists {
			httpError = http.StatusConflict
		}
		return &errors.HTTP{Code: httpError, Message: err.Error()}
	}
	fmt.Fprint(w, "success")
	return nil
}

func serviceUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var y serviceYaml
	err = yaml.Unmarshal(body, &y)
	if err != nil {
		return err
	}
	err = y.validate()
	if err != nil {
		return err
	}
	s, err := getService(y.Id)
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
	rec.Log(t.GetUserName(), "update-service", y.Id, y.Endpoint)
	s.Endpoint = y.Endpoint
	s.Password = y.Password
	s.Username = y.Username
	if err = s.Update(); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

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
		msg := "This service cannot be removed because it has instances.\nPlease remove these instances before removing the service."
		return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	err = s.Delete()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
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
			return &errors.HTTP{Code: http.StatusNotFound, Message: "Team not found"}
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
			return &errors.HTTP{Code: http.StatusNotFound, Message: "Team not found"}
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
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return s.Update()
}

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
