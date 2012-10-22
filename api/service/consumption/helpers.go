// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package consumption

import (
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/api/service"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"labix.org/v2/mgo/bson"
	"net/http"
)

func GetServiceOrError(name string, u *auth.User) (service.Service, error) {
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

func GetServiceInstanceOrError(name string, u *auth.User) (service.ServiceInstance, error) {
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

func ServiceAndServiceInstancesByTeams(u *auth.User) []service.ServiceModel {
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
