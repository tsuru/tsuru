// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/rec"
	"github.com/globocom/tsuru/service"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"net/http"
)

func createServiceInstance(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var body map[string]string
	err = json.Unmarshal(b, &body)
	if err != nil {
		return err
	}
	serviceName := body["service_name"]
	user, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(user.Email, "create-service-instance", string(b))
	srv, err := getServiceOrError(serviceName, user)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return service.CreateServiceInstance(body["name"], &srv, user)
}

func removeServiceInstance(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	name := r.URL.Query().Get(":name")
	rec.Log(u.Email, "remove-service-instance", name)
	si, err := getServiceInstanceOrError(name, u)
	if err != nil {
		return err
	}
	err = service.DeleteInstance(si)
	if err != nil {
		return err
	}
	w.Write([]byte("service instance successfuly removed"))
	return nil
}

func serviceInstances(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "list-service-instances")
	services, _ := service.GetServicesByTeamKindAndNoRestriction("teams", u)
	sInstances, _ := service.GetServiceInstancesByServicesAndTeams(services, u)
	result := make([]service.ServiceModel, len(services))
	for i, s := range services {
		result[i].Service = s.Name
		for _, si := range sInstances {
			if si.ServiceName == s.Name {
				result[i].Instances = append(result[i].Instances, si.Name)
			}
		}
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	n, err := w.Write(body)
	if n != len(body) {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: "Failed to write the response body."}
	}
	return err
}

func serviceInstance(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	instance, err := getServiceInstanceOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(instance)
}

func serviceInstanceStatus(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	siName := r.URL.Query().Get(":instance")
	si, err := getServiceInstanceOrError(siName, u)
	if err != nil {
		return err
	}
	rec.Log(u.Email, "service-instance-status", siName)
	var b string
	if b, err = si.Status(); err != nil {
		msg := fmt.Sprintf("Could not retrieve status of service instance, error: %s", err.Error())
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: msg}
	}
	b = fmt.Sprintf(`Service instance "%s" is %s`, siName, b)
	n, err := w.Write([]byte(b))
	if n != len(b) {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: "Failed to write response body"}
	}
	return nil
}

func serviceInfo(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	serviceName := r.URL.Query().Get(":name")
	rec.Log(u.Email, "service-info", serviceName)
	_, err = getServiceOrError(serviceName, u)
	if err != nil {
		return err
	}
	instances := []service.ServiceInstance{}
	teams, err := u.Teams()
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	teamsNames := auth.GetTeamsNames(teams)
	q := bson.M{"service_name": serviceName, "teams": bson.M{"$in": teamsNames}}
	err = conn.ServiceInstances().Find(q).All(&instances)
	if err != nil {
		return err
	}
	b, err := json.Marshal(instances)
	if err != nil {
		return nil
	}
	w.Write(b)
	return nil
}

func serviceDoc(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	sName := r.URL.Query().Get(":name")
	rec.Log(u.Email, "service-doc", sName)
	s, err := getServiceOrError(sName, u)
	if err != nil {
		return err
	}
	w.Write([]byte(s.Doc))
	return nil
}

func getServiceOrError(name string, u *auth.User) (service.Service, error) {
	s := service.Service{Name: name}
	err := s.Get()
	if err != nil {
		return s, &errors.HTTP{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !s.IsRestricted {
		return s, nil
	}
	if !auth.CheckUserAccess(s.Teams, u) {
		msg := "This user does not have access to this service"
		return s, &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	return s, err
}

func getServiceInstanceOrError(name string, u *auth.User) (*service.ServiceInstance, error) {
	si, err := service.GetServiceInstance(name, u)
	if err != nil {
		switch err {
		case service.ErrServiceInstanceNotFound:
			return nil, &errors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		case service.ErrAccessNotAllowed:
			return nil, &errors.HTTP{
				Code:    http.StatusForbidden,
				Message: err.Error(),
			}
		default:
			return nil, err
		}
	}
	return si, nil
}
