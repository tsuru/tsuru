// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/router"
	bindTypes "github.com/tsuru/tsuru/types/bind"
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
	defaultRouter, err := router.Default(context.TODO())
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
		envsMap := map[bindTypes.ServiceEnvVar]struct{}{}
		for _, sEnv := range a.ServiceEnvs {
			envsMap[sEnv] = struct{}{}
		}
		var serviceNames []string
		for serviceName := range data {
			serviceNames = append(serviceNames, serviceName)
		}
		sort.Strings(serviceNames)
		var serviceEnvs []bindTypes.ServiceEnvVar
		for _, serviceName := range serviceNames {
			instances := data[serviceName]
			for _, instance := range instances {
				for k, v := range instance.Envs {
					toAppendEnv := bindTypes.ServiceEnvVar{
						ServiceName:  serviceName,
						InstanceName: instance.InstanceName,
						EnvVar:       bindTypes.EnvVar{Name: k, Value: v},
					}
					if _, ok := envsMap[toAppendEnv]; !ok {
						serviceEnvs = append(serviceEnvs, toAppendEnv)
					}
				}
			}
		}
		err = conn.Apps().Update(bson.M{"name": a.Name}, bson.M{
			"$push": bson.M{"serviceenvs": bson.M{"$each": serviceEnvs, "$position": 0}},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

type appWithPlanID struct {
	Name string
	Plan planWithID
}

type planWithID struct {
	ID   string `bson:"_id"`
	Name string
}

func MigrateAppPlanIDToPlanName() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	iter := conn.Apps().Find(nil).Iter()
	var a appWithPlanID
	for iter.Next(&a) {
		if a.Plan.Name != "" || a.Plan.ID == "" {
			continue
		}
		err = conn.Apps().Update(bson.M{"name": a.Name}, bson.M{"$set": bson.M{"plan.name": a.Plan.ID}})
		if err != nil {
			return err
		}
	}
	return nil
}
