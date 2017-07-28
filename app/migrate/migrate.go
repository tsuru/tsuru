// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/router"
	"gopkg.in/mgo.v2/bson"
)

type AppWithPlanRouter struct {
	Name   string
	Plan   PlanWithRouter
	Router string
}

type PlanWithRouter struct {
	Router string
}

func MigrateAppPlanRouterToRouter() error {
	defaultRouter, err := router.Default()
	if err != nil {
		if err == router.ErrDefaultRouterNotFound {
			fmt.Println("A default router must be configured in order to run this migration.")
			fmt.Println("To fix this, either set the \"docker:router\" or \"router:<router_name>:default\" configs.")
		}
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	iter := conn.Apps().Find(nil).Iter()
	var app AppWithPlanRouter
	for iter.Next(&app) {
		if app.Router != "" {
			continue
		}
		r := defaultRouter
		if app.Plan.Router != "" {
			r = app.Plan.Router
		}
		err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"router": r}})
		if err != nil {
			return err
		}
	}
	return nil
}

func MigrateAppTsuruServicesVarToServiceEnvs() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	iter := conn.Apps().Find(nil).Iter()
	var a app.App
	for iter.Next(&a) {
		serviceEnvVar := a.Env[app.TsuruServicesEnvVar]
		if serviceEnvVar.Value == "" {
			continue
		}
		var data map[string][]struct {
			InstanceName string            `json:"instance_name"`
			Envs         map[string]string `json:"envs"`
		}
		err = json.Unmarshal([]byte(serviceEnvVar.Value), &data)
		if err != nil {
			return err
		}
		var serviceNames []string
		for serviceName := range data {
			serviceNames = append(serviceNames, serviceName)
		}
		sort.Strings(serviceNames)
		var serviceEnvs []bind.ServiceEnvVar
		for _, serviceName := range serviceNames {
			instances := data[serviceName]
			for _, instance := range instances {
				for k, v := range instance.Envs {
					serviceEnvs = append(serviceEnvs, bind.ServiceEnvVar{
						ServiceName:  serviceName,
						InstanceName: instance.InstanceName,
						EnvVar:       bind.EnvVar{Name: k, Value: v},
					})
				}
			}
		}
		err = conn.Apps().Update(bson.M{"name": a.Name}, bson.M{
			"$push":  bson.M{"serviceenvs": bson.M{"$each": serviceEnvs, "$position": 0}},
			"$unset": bson.M{"env." + app.TsuruServicesEnvVar: ""},
		})
		if err != nil {
			return err
		}
	}
	return nil
}
