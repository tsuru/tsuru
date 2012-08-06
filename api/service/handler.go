package service

import (
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
)

type ServiceModel struct {
	Service   string
	Instances []string
}

func ServiceAndServiceInstancesByTeams(teamKind string, u *auth.User) []ServiceModel {
	var teams []auth.Team
	q := bson.M{"users": u.Email}
	db.Session.Teams().Find(q).Select(bson.M{"_id": 1}).All(&teams)
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
