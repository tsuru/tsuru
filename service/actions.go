// Copyright 2015 tsuru authors. All rights reserved.
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

// createServiceInstance is an action that calls the service endpoint
// to create a service instance.
//
// The first argument in the context must be a Service.
// The second argument in the context must be a ServiceInstance.
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
		if err := si.update(nil); err != nil {
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
		if err := si.update(nil); err != nil {
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
				if err != nil && err != ErrUnitAlreadyBound {
					errChan <- err
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

type unbindPipelineArgs struct {
	app             bind.App
	writer          io.Writer
	serviceInstance *ServiceInstance
}

var unbindUnits = action.Action{
	Name: "unbind-units",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*unbindPipelineArgs)
		if args == nil {
			return nil, stderrors.New("invalid arguments for pipeline, expected *unbindPipelineArgs")
		}
		var wg sync.WaitGroup
		si := args.serviceInstance
		units := args.app.GetUnits()
		errCh := make(chan error, len(units))
		unbindedCh := make(chan bind.Unit, len(units))
		for i := range units {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				unit := units[i]
				err := si.UnbindUnit(args.app, unit)
				if err == nil || err == ErrUnitNotBound {
					unbindedCh <- unit
				} else {
					errCh <- err
				}
			}(i)
		}
		wg.Wait()
		close(errCh)
		close(unbindedCh)
		if err := <-errCh; err != nil {
			for unit := range unbindedCh {
				rebindErr := si.BindUnit(args.app, unit)
				if rebindErr != nil {
					log.Errorf("[unbind-units forward] failed to rebind unit after error: %s", rebindErr)
				}
			}
			return nil, err
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*unbindPipelineArgs)
		units := args.app.GetUnits()
		for _, unit := range units {
			err := args.serviceInstance.BindUnit(args.app, unit)
			if err != nil {
				log.Errorf("[unbind-units backward] failed to rebind unit in rollback: %s", err)
			}
		}
	},
}

var unbindAppDB = action.Action{
	Name: "unbind-app-db",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*unbindPipelineArgs)
		if args == nil {
			return nil, stderrors.New("invalid arguments for pipeline, expected *unbindPipelineArgs")
		}
		err := args.serviceInstance.update(bson.M{"$pull": bson.M{"apps": args.app.GetName()}})
		if err != nil {
			return nil, err
		}
		args.serviceInstance.RemoveApp(args.app.GetName())
		return nil, err
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*unbindPipelineArgs)
		args.serviceInstance.AddApp(args.app.GetName())
		err := args.serviceInstance.update(bson.M{"$addToSet": bson.M{"apps": args.app.GetName()}})
		if err != nil {
			log.Errorf("[unbind-app-db backward] failed to rebind app in db: %s", err)
		}
	},
}

var unbindAppEndpoint = action.Action{
	Name: "unbind-app-endpoint",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*unbindPipelineArgs)
		if args == nil {
			return nil, stderrors.New("invalid arguments for pipeline, expected *unbindPipelineArgs")
		}
		if endpoint, err := args.serviceInstance.Service().getClient("production"); err == nil {
			err := endpoint.UnbindApp(args.serviceInstance, args.app)
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*unbindPipelineArgs)
		if endpoint, err := args.serviceInstance.Service().getClient("production"); err == nil {
			_, err := endpoint.BindApp(args.serviceInstance, args.app)
			if err != nil {
				log.Errorf("[unbind-app-endpoint backward] failed to rebind app in endpoint: %s", err)
			}
		}
	},
}

var removeBindedEnvs = action.Action{
	Name: "remove-binded-envs",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*unbindPipelineArgs)
		if args == nil {
			return nil, stderrors.New("invalid arguments for pipeline, expected *unbindPipelineArgs")
		}
		si := args.serviceInstance
		instance := bind.ServiceInstance{Name: si.Name, Envs: make(map[string]string)}
		for k, envVar := range args.app.InstanceEnv(si.Name) {
			instance.Envs[k] = envVar.Value
		}
		return nil, args.app.RemoveInstance(si.ServiceName, instance, args.writer)
	},
	Backward: func(ctx action.BWContext) {
	},
}
