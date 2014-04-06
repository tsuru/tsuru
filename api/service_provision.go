// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/rec"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/v1/yaml"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"net/http"
)

type serviceYaml struct {
	Id       string
	Endpoint map[string]string
}

func serviceList(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "list-services")
	results := servicesAndInstancesByOwner(u)
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

func serviceCreate(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var sy serviceYaml
	err = yaml.Unmarshal(body, &sy)
	if err != nil {
		return err
	}
	if _, ok := sy.Endpoint["production"]; !ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "You must provide a production endpoint in the manifest file."}
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "create-service", sy.Id, sy.Endpoint)
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var teams []auth.Team
	err = conn.Teams().Find(bson.M{"users": u.Email}).All(&teams)
	if err != nil {
		return err
	}
	if len(teams) == 0 {
		msg := "In order to create a service, you should be member of at least one team"
		return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	n, err := conn.Services().Find(bson.M{"_id": sy.Id}).Count()
	if err != nil {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	if n != 0 {
		msg := fmt.Sprintf("Service with name %s already exists.", sy.Id)
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: msg}
	}
	s := service.Service{
		Name:       sy.Id,
		Endpoint:   sy.Endpoint,
		OwnerTeams: auth.GetTeamsNames(teams),
	}
	err = s.Create()
	if err != nil {
		return err
	}
	fmt.Fprint(w, "success")
	return nil
}

func serviceUpdate(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var y serviceYaml
	yaml.Unmarshal(body, &y)
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "update-service", y.Id, y.Endpoint)
	s, err := getServiceByOwner(y.Id, u)
	if err != nil {
		return err
	}
	s.Endpoint = y.Endpoint
	if err = s.Update(); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func serviceDelete(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "delete-service", r.URL.Query().Get(":name"))
	s, err := getServiceByOwner(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	n, err := conn.ServiceInstances().Find(bson.M{"service_name": s.Name}).Count()
	if err != nil {
		return err
	}
	if n > 0 {
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

func getServiceAndTeam(serviceName string, teamName string, u *auth.User) (*service.Service, *auth.Team, error) {
	service := &service.Service{Name: serviceName}
	err := service.Get()
	if err != nil {
		return nil, nil, &errors.HTTP{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !auth.CheckUserAccess(service.Teams, u) {
		msg := "This user does not have access to this service"
		return nil, nil, &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	t := new(auth.Team)
	conn, err := db.Conn()
	if err != nil {
		return nil, nil, err
	}
	err = conn.Teams().Find(bson.M{"_id": teamName}).One(t)
	if err != nil {
		return nil, nil, &errors.HTTP{Code: http.StatusNotFound, Message: "Team not found"}
	}
	return service, t, nil
}

func grantServiceAccess(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	serviceName := r.URL.Query().Get(":service")
	teamName := r.URL.Query().Get(":team")
	rec.Log(u.Email, "grant-service-access", "service="+serviceName, "team="+teamName)
	service, team, err := getServiceAndTeam(serviceName, teamName, u)
	if err != nil {
		return err
	}
	err = service.GrantAccess(team)
	if err != nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	return conn.Services().Update(bson.M{"_id": service.Name}, service)
}

func revokeServiceAccess(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	serviceName := r.URL.Query().Get(":service")
	teamName := r.URL.Query().Get(":team")
	rec.Log(u.Email, "revoke-service-access", "service="+serviceName, "team="+teamName)
	service, team, err := getServiceAndTeam(serviceName, teamName, u)
	if err != nil {
		return err
	}
	if len(service.Teams) < 2 {
		msg := "You can not revoke the access from this team, because it is the unique team with access to this service, and a service can not be orphaned"
		return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	err = service.RevokeAccess(team)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	return conn.Services().Update(bson.M{"_id": service.Name}, service)
}

func serviceAddDoc(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	rec.Log(u.Email, "service-add-doc", r.URL.Query().Get(":name"), string(body))
	s, err := getServiceByOwner(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	s.Doc = string(body)
	if err = s.Update(); err != nil {
		return err
	}
	return nil
}

func getServiceByOwner(name string, u *auth.User) (service.Service, error) {
	s := service.Service{Name: name}
	err := s.Get()
	if err != nil {
		return s, &errors.HTTP{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !auth.CheckUserAccess(s.OwnerTeams, u) {
		msg := "This user does not have access to this service"
		return s, &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	return s, err
}

func servicesAndInstancesByOwner(u *auth.User) []service.ServiceModel {
	services, _ := service.GetServicesByOwnerTeams("owner_teams", u)
	sInstances, _ := service.GetServiceInstancesByServices(services)
	results := make([]service.ServiceModel, len(services))
	for i, s := range services {
		results[i].Service = s.Name
		for _, si := range sInstances {
			if si.ServiceName == s.Name {
				results[i].Instances = append(results[i].Instances, si.Name)
			}
		}
	}
	return results
}
