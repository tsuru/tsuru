package service

import (
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"labix.org/v2/mgo/bson"
	"net/http"
)

type ServiceModel struct {
	Service   string
	Instances []string
}

func GetServiceOrError(name string, u *auth.User) (Service, error) {
	s := Service{Name: name}
	err := s.Get()
	if err != nil {
		return s, &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !auth.CheckUserAccess(s.OwnerTeams, u) {
		msg := "This user does not have access to this service"
		return s, &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	return s, err
}

func ServiceAndServiceInstancesByTeams(teamKind string, u *auth.User) []ServiceModel {
	var teams []auth.Team
	q := bson.M{"users.email": u.Email}
	db.Session.Teams().Find(q).Select(bson.M{"name": 1}).All(&teams)
	var services []Service
	q = bson.M{teamKind: bson.M{"$in": auth.GetTeamsNames(teams)}}
	db.Session.Services().Find(q).Select(bson.M{"name": 1}).All(&services)
	var sInsts []ServiceInstance
	q = bson.M{"service_name": bson.M{"$in": GetServicesNames(services)}}
	db.Session.ServiceInstances().Find(q).Select(bson.M{"name": 1, "service_name": 1}).All(&sInsts)
	results := make([]ServiceModel, len(services))
	for i, s := range services {
		results[i].Service = s.Name
		for _, si := range sInsts {
			if si.ServiceName == s.Name {
				results[i].Instances = append(results[i].Instances, si.Name)
			}
		}
	}
	return results
}
