// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	stderrors "errors"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"labix.org/v2/mgo/bson"
	"net/http"
)

// createServiceInstance in an action that calls the service endpoint
// to creates the service instance.
//
// The first argument in the context must be an Service.
// The second argument in the context must be an ServiceInstance.
var createServiceInstance = action.Action{
	Name: "create-service-instance",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		service, ok := ctx.Params[0].(Service)
		if !ok {
			return nil, stderrors.New("First parameter must be a Service.")
		}
		endpoint, err := service.getClient("production")
		if err != nil {
			return nil, err
		}
		instance, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			return nil, stderrors.New("Second parameter must be a ServiceInstance.")
		}
		err = endpoint.Create(&instance)
		if err != nil {
			return nil, err
		}
		return instance, nil
	},
	Backward: func(ctx action.BWContext) {
		service, ok := ctx.Params[0].(Service)
		if !ok {
			return
		}
		endpoint, err := service.getClient("production")
		if err != nil {
			return
		}
		instance, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			return
		}
		endpoint.Destroy(&instance)
	},
	MinParams: 2,
}

// insertServiceInstance is an action that inserts an instance in the database.
//
// The first argument in the context must be a Service Instance.
var insertServiceInstance = action.Action{
	Name: "insert-service-instance",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		instance, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			return nil, stderrors.New("Second parameter must be a ServiceInstance.")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		err = conn.ServiceInstances().Insert(&instance)
		if err != nil {
			return nil, err
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		instance, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			return
		}
		conn, err := db.Conn()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.ServiceInstances().Remove(bson.M{"name": instance.Name})
	},
	MinParams: 2,
}

var addAppToServiceInstance = action.Action{
	Name: "add-app-to-service-instance",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		si, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			return nil, stderrors.New("Second parameter must be a ServiceInstance.")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		if err := conn.ServiceInstances().Find(bson.M{"name": si.Name}).One(&si); err != nil {
			return nil, err
		}
		a, ok := ctx.Params[0].(provision.App)
		if !ok {
			return nil, stderrors.New("First parameter must be a provision.App.")
		}
		if err := si.AddApp(a.GetName()); err != nil {
			return nil, &errors.HTTP{Code: http.StatusConflict, Message: "This app is already bound to this service instance."}
		}
		if err := si.update(); err != nil {
			return nil, err
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		si, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			log.Error("Second parameter must be a ServiceInstance.")
		}
		a, ok := ctx.Params[0].(provision.App)
		if !ok {
			log.Error("First parameter must be a provision.App.")
		}
		si.RemoveApp(a.GetName())
		if err := si.update(); err != nil {
			log.Errorf("Could not remove app from service instance: %s", err.Error())
		}
	},
	MinParams: 2,
}
