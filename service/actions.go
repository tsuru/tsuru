// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	stderrors "errors"
	"io"
	"net/http"
	"sync"

	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2/bson"
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
		user, ok := ctx.Params[2].(string)
		if !ok {
			return nil, stderrors.New("Third parameter must be a string.")
		}
		err = endpoint.Create(&instance, user)
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
		a, ok := ctx.Params[0].(bind.App)
		if !ok {
			return nil, stderrors.New("First parameter must be a bind.App.")
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
		a, ok := ctx.Params[0].(bind.App)
		if !ok {
			log.Error("First parameter must be a bind.App.")
		}
		si.RemoveApp(a.GetName())
		if err := si.update(); err != nil {
			log.Errorf("Could not remove app from service instance: %s", err.Error())
		}
	},
	MinParams: 2,
}

var setBindAppAction = action.Action{
	Name: "set-environ-variables-to-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		si, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			msg := "Second parameter must be a ServiceInstance."
			log.Error(msg)
			return nil, stderrors.New(msg)
		}
		app, ok := ctx.Params[0].(bind.App)
		if !ok {
			msg := "First parameter must be a bind.App."
			log.Error(msg)
			return nil, stderrors.New(msg)
		}
		endpoint, err := si.Service().getClient("production")
		if err != nil {
			return nil, err
		}
		return endpoint.BindApp(&si, app)
	},
	Backward: func(ctx action.BWContext) {
		app, ok := ctx.Params[0].(bind.App)
		if !ok {
			log.Error("First parameter must be a bind.App.")
			return
		}
		si, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			log.Error("Second parameter must be a ServiceInstance.")
			return
		}
		endpoint, err := si.Service().getClient("production")
		if err != nil {
			log.Errorf("Could not get endpoint: %s.", err.Error())
			return
		}
		endpoint.UnbindApp(&si, app)
	},
}

var setTsuruServices = action.Action{
	Name: "set-TSURU_SERVICES-env-var",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var writer io.Writer
		if len(ctx.Params) > 2 && ctx.Params[2] != nil {
			var ok bool
			writer, ok = ctx.Params[2].(io.Writer)
			if !ok {
				msg := "Third parameter must be a io.Writer."
				log.Error(msg)
				return nil, stderrors.New(msg)
			}
		}
		si, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			msg := "Second parameter must be a ServiceInstance."
			log.Error(msg)
			return nil, stderrors.New(msg)
		}
		app, ok := ctx.Params[0].(bind.App)
		if !ok {
			msg := "First parameter must be a bind.App."
			log.Error(msg)
			return nil, stderrors.New(msg)
		}
		instance := bind.ServiceInstance{
			Name: si.Name,
			Envs: ctx.Previous.(map[string]string),
		}
		return instance, app.AddInstance(si.ServiceName, instance, writer)
	},
	Backward: func(ctx action.BWContext) {
		var writer io.Writer
		if len(ctx.Params) > 2 && ctx.Params[2] != nil {
			var ok bool
			writer, ok = ctx.Params[2].(io.Writer)
			if !ok {
				log.Error("Third parameter must be a io.Writer.")
				return
			}
		}
		si, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			log.Error("Second parameter must be a ServiceInstance.")
			return
		}
		app, ok := ctx.Params[0].(bind.App)
		if !ok {
			log.Error("First parameter must be a bind.App.")
			return
		}
		instance := ctx.FWResult.(bind.ServiceInstance)
		app.RemoveInstance(si.ServiceName, instance, writer)
	},
}

var bindUnitsToServiceInstance = action.Action{
	Name: "bind-units-to-service-instance",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		si, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			msg := "Second parameter must be a ServiceInstance."
			log.Error(msg)
			return nil, stderrors.New(msg)
		}
		app, ok := ctx.Params[0].(bind.App)
		if !ok {
			msg := "First parameter must be a bind.App."
			log.Error(msg)
			return nil, stderrors.New(msg)
		}
		units := app.GetUnits()
		if len(units) == 0 {
			return nil, nil
		}
		errChan := make(chan error, len(units)+1)
		var wg sync.WaitGroup
		wg.Add(len(units))
		for _, unit := range units {
			go func(unit bind.Unit) {
				defer wg.Done()
				err := si.BindUnit(app, unit)
				if err != nil {
					errChan <- err
					return
				}
			}(unit)
		}
		wg.Wait()
		close(errChan)
		if err, ok := <-errChan; ok {
			log.Error(err.Error())
			return nil, err
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}
