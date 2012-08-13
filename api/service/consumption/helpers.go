package consumption

import (
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"labix.org/v2/mgo/bson"
	"net/http"
)

func GetServiceOr404(name string) (service.Service, error) {
	s := service.Service{Name: name}
	err := s.Get()
	if err != nil {
		return s, &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
	}
	return s, nil
}

func GetServiceOrError(name string, u *auth.User) (service.Service, error) {
	s, err := GetServiceOr404(name)
	if err != nil {
		return s, err
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

func GetServiceInstanceOr404(name string) (service.ServiceInstance, error) {
	var si service.ServiceInstance
	err := db.Session.ServiceInstances().Find(bson.M{"_id": name}).One(&si)
	if err != nil {
		return si, &errors.Http{Code: http.StatusNotFound, Message: "Service instance not found"}
	}
	return si, nil
}

func GetServiceInstanceOrError(name string, u *auth.User) (service.ServiceInstance, error) {
	si, err := GetServiceInstanceOr404(name)
	if err != nil {
		return si, err
	}
	if !auth.CheckUserAccess(si.Teams, u) {
		msg := "This user does not have access to this service instance"
		return si, &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	return si, nil
}

type ServiceModel struct {
	Service   string
	Instances []string
}

func ServiceAndServiceInstancesByTeams(teamKind string, u *auth.User) []ServiceModel {
	var teams []auth.Team
	q := bson.M{"users": u.Email}
	db.Session.Teams().Find(q).Select(bson.M{"_id": 1}).All(&teams)
	teamsNames := auth.GetTeamsNames(teams)
	var services []service.Service
	q = bson.M{"$or": []bson.M{
		bson.M{
			teamKind: bson.M{"$in": teamsNames},
		},
		bson.M{"is_restricted": false},
	},
	}
	db.Session.Services().Find(q).Select(bson.M{"name": 1}).All(&services)
	var sInsts []service.ServiceInstance
	q = bson.M{"service_name": bson.M{"$in": service.GetServicesNames(services)}, "teams": bson.M{"$in": teamsNames}}
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
