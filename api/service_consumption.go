// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/service"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"net/http"
)

func CreateInstanceHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	log.Print("Receiving request to create a service instance")
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Print("Got error while reading request body:")
		log.Print(err.Error())
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	var sJson map[string]string
	err = json.Unmarshal(b, &sJson)
	if err != nil {
		log.Print("Got a problem while unmarshalling request's json:")
		log.Print(err.Error())
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	var s service.Service
	err = validateInstanceForCreation(&s, sJson, u)
	if err != nil {
		log.Print("Got error while validation:")
		log.Print(err.Error())
		return err
	}
	var teamNames []string
	teams, err := u.Teams()
	if err != nil {
		return err
	}
	for _, t := range teams {
		if s.HasTeam(&t) || !s.IsRestricted {
			teamNames = append(teamNames, t.Name)
		}
	}
	si := service.ServiceInstance{
		Name:        sJson["name"],
		ServiceName: sJson["service_name"],
		Teams:       teamNames,
	}
	if err = s.ProductionEndpoint().Create(&si); err != nil {
		log.Print("Error while calling create action from service api.")
		log.Print(err.Error())
		return err
	}
	err = si.Create()
	if err != nil {
		return err
	}
	fmt.Fprint(w, "success")
	return nil
}

func validateInstanceForCreation(s *service.Service, sJson map[string]string, u *auth.User) error {
	err := db.Session.Services().Find(bson.M{"_id": sJson["service_name"], "status": bson.M{"$ne": "deleted"}}).One(&s)
	if err != nil {
		msg := err.Error()
		if msg == "not found" {
			msg = fmt.Sprintf("Service %s does not exist.", sJson["service_name"])
		}
		return &errors.Http{Code: http.StatusNotFound, Message: msg}
	}
	_, err = getServiceOrError(sJson["service_name"], u)
	if err != nil {
		return err
	}
	return nil
}

func RemoveServiceInstanceHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	name := r.URL.Query().Get(":name")
	si, err := getServiceInstanceOrError(name, u)
	if err != nil {
		return err
	}
	if len(si.Apps) > 0 {
		msg := "This service instance has binded apps. Unbind them before removing it"
		return &errors.Http{Code: http.StatusInternalServerError, Message: msg}
	}
	if err = si.Service().ProductionEndpoint().Destroy(&si); err != nil {
		//TODO(flaviamissi): return err
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	err = db.Session.ServiceInstances().Remove(bson.M{"name": name})
	if err != nil {
		return err
	}
	w.Write([]byte("service instance successfuly removed"))
	return nil
}

func ServicesInstancesHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	response := serviceAndServiceInstancesByTeams(u)
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	n, err := w.Write(body)
	if n != len(body) {
		return &errors.Http{Code: http.StatusInternalServerError, Message: "Failed to write the response body."}
	}
	return err
}

func ServiceInstanceStatusHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	// #TODO (flaviamissi) should check if user has access to service
	// just call GetServiceInstanceOrError should be enough
	siName := r.URL.Query().Get(":instance")
	var si service.ServiceInstance
	if siName == "" {
		return &errors.Http{Code: http.StatusBadRequest, Message: "Service instance name not provided."}
	}
	err := db.Session.ServiceInstances().Find(bson.M{"name": siName}).One(&si)
	if err != nil {
		msg := fmt.Sprintf("Service instance does not exists, error: %s", err.Error())
		return &errors.Http{Code: http.StatusInternalServerError, Message: msg}
	}
	s := si.Service()
	var b string
	if b, err = s.ProductionEndpoint().Status(&si); err != nil {
		msg := fmt.Sprintf("Could not retrieve status of service instance, error: %s", err.Error())
		return &errors.Http{Code: http.StatusInternalServerError, Message: msg}
	}
	b = fmt.Sprintf(`Service instance "%s" is %s`, siName, b)
	n, err := w.Write([]byte(b))
	if n != len(b) {
		return &errors.Http{Code: http.StatusInternalServerError, Message: "Failed to write response body"}
	}
	return nil
}

func ServiceInfoHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	serviceName := r.URL.Query().Get(":name")
	_, err := getServiceOrError(serviceName, u)
	if err != nil {
		return err
	}
	instances := []service.ServiceInstance{}
	teams, err := u.Teams()
	if err != nil {
		return err
	}
	teamsNames := auth.GetTeamsNames(teams)
	err = db.Session.ServiceInstances().Find(bson.M{"service_name": serviceName, "teams": bson.M{"$in": teamsNames}}).All(&instances)
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

func Doc(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	sName := r.URL.Query().Get(":name")
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
		return s, &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !s.IsRestricted {
		return s, nil
	}
	if !auth.CheckUserAccess(s.Teams, u) {
		msg := "This user does not have access to this service"
		return s, &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	return s, err
}

func getServiceInstanceOrError(name string, u *auth.User) (service.ServiceInstance, error) {
	var si service.ServiceInstance
	err := db.Session.ServiceInstances().Find(bson.M{"name": name}).One(&si)
	if err != nil {
		return si, &errors.Http{Code: http.StatusNotFound, Message: "Service instance not found"}
	}
	if !auth.CheckUserAccess(si.Teams, u) {
		msg := "This user does not have access to this service instance"
		return si, &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	return si, nil
}

func serviceAndServiceInstancesByTeams(u *auth.User) []service.ServiceModel {
	services, _ := service.GetServicesByTeamKindAndNoRestriction("teams", u)
	sInstances, _ := service.GetServiceInstancesByServicesAndTeams(services, u)
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
