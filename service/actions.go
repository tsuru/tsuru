// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"io"
	"sort"
	"sync"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
)

// notifyCreateServiceInstance is an action that calls the service endpoint
// to create a service instance.
//
// The first argument in the context must be a Service.
// The second argument in the context must be a ServiceInstance.
// The third argument in the context must be a request ID.
var notifyCreateServiceInstance = action.Action{
	Name: "notify-create-service-instance",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		service, ok := ctx.Params[0].(Service)
		if !ok {
			return nil, errors.New("First parameter must be a Service.")
		}
		endpoint, err := service.getClient("production")
		if err != nil {
			return nil, err
		}
		instance, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			return nil, errors.New("Second parameter must be a ServiceInstance.")
		}
		evt, ok := ctx.Params[2].(*event.Event)
		if !ok {
			return nil, errors.New("Third parameter must be an event.")
		}
		requestID, ok := ctx.Params[3].(string)
		if !ok {
			return nil, errors.New("RequestID should be a string.")
		}
		err = endpoint.Create(&instance, evt, requestID)
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
		evt, ok := ctx.Params[2].(*event.Event)
		if !ok {
			return
		}
		requestID, ok := ctx.Params[3].(string)
		if !ok {
			return
		}
		endpoint.Destroy(&instance, evt, requestID)
	},
	MinParams: 3,
}

// createServiceInstance is an action that inserts an instance in the database.
//
// The second argument in the context must be a Service Instance.
var createServiceInstance = action.Action{
	Name: "create-service-instance",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		instance, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			return nil, errors.New("Second parameter must be a ServiceInstance.")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return nil, conn.ServiceInstances().Insert(&instance)
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
		conn.ServiceInstances().Remove(bson.M{"name": instance.Name, "service_name": instance.ServiceName})
	},
	MinParams: 2,
}

// updateServiceInstance is an action that updates an instance in the database.
//
// The second argument in the context must be a Service Instance with the current attributes.
// The third argument in the context must be a Service Instance with the updated attributes.
var updateServiceInstance = action.Action{
	Name: "update-service-instance",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		instance, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			return nil, errors.New("Second parameter must be a ServiceInstance.")
		}
		updateData, ok := ctx.Params[2].(ServiceInstance)
		if !ok {
			return nil, errors.New("Third parameter must be a ServiceInstance.")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return nil, conn.ServiceInstances().Update(
			bson.M{"name": instance.Name, "service_name": instance.ServiceName},
			bson.M{
				"$set": bson.M{
					"description": updateData.Description,
					"tags":        updateData.Tags,
					"teamowner":   updateData.TeamOwner,
				},
				"$addToSet": bson.M{
					"teams": updateData.TeamOwner,
				},
			},
		)
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
		conn.ServiceInstances().Update(
			bson.M{"name": instance.Name, "service_name": instance.ServiceName},
			bson.M{
				"$set": bson.M{
					"description": instance.Description,
					"tags":        instance.Tags,
					"teamowner":   instance.TeamOwner,
					"teams":       instance.Teams,
				},
			},
		)
	},
	MinParams: 3,
}

// notifyUpdateServiceInstance is an action that calls the service endpoint
// to update a service instance.
//
// The first argument in the context must be a Service.
// The second argument in the context must be a ServiceInstance.
// The forth argument in the context must be a request ID.
var notifyUpdateServiceInstance = action.Action{
	Name: "notify-update-service-instance",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		service, ok := ctx.Params[0].(Service)
		if !ok {
			return nil, errors.New("First parameter must be a Service.")
		}
		endpoint, err := service.getClient("production")
		if err != nil {
			return nil, err
		}
		instance, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			return nil, errors.New("Second parameter must be a ServiceInstance.")
		}
		evt, ok := ctx.Params[3].(*event.Event)
		if !ok {
			return nil, errors.New("Third parameter must be an event.")
		}
		requestID, ok := ctx.Params[4].(string)
		if !ok {
			return nil, errors.New("RequestID should be a string.")
		}
		err = endpoint.Update(&instance, evt, requestID)
		if err != nil {
			return nil, err
		}
		return instance, nil
	},
	Backward:  func(ctx action.BWContext) {},
	MinParams: 4,
}

type bindPipelineArgs struct {
	app             bind.App
	writer          io.Writer
	serviceInstance *ServiceInstance
	shouldRestart   bool
	event           *event.Event
	requestID       string
}

var bindAppDBAction = &action.Action{
	Name: "bind-app-db",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindPipelineArgs.")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		si := args.serviceInstance
		updateOp := bson.M{"$addToSet": bson.M{"apps": args.app.GetName()}}
		err = conn.ServiceInstances().Update(bson.M{"name": si.Name, "service_name": si.ServiceName, "apps": bson.M{"$ne": args.app.GetName()}}, updateOp)
		if err != nil {
			if err == mgo.ErrNotFound {
				return nil, ErrAppAlreadyBound
			}
			return nil, err
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		if err := args.serviceInstance.updateData(bson.M{"$pull": bson.M{"apps": args.app.GetName()}}); err != nil {
			log.Errorf("[bind-app-db backward] could not remove app from service instance: %s", err)
		}
	},
	MinParams: 1,
}

var bindAppEndpointAction = &action.Action{
	Name: "bind-app-endpoint",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindPipelineArgs.")
		}
		endpoint, err := args.serviceInstance.Service().getClient("production")
		if err != nil {
			return nil, err
		}
		return endpoint.BindApp(args.serviceInstance, args.app, args.event, args.requestID)
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		endpoint, err := args.serviceInstance.Service().getClient("production")
		if err != nil {
			log.Errorf("[bind-app-endpoint backward] could not get endpoint: %s", err)
			return
		}
		err = endpoint.UnbindApp(args.serviceInstance, args.app, args.event, args.requestID)
		if err != nil {
			log.Errorf("[bind-app-endpoint backward] failed to unbind unit: %s", err)
		}
	},
	MinParams: 1,
}

var setBoundEnvsAction = &action.Action{
	Name: "set-bound-envs",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindPipelineArgs.")
		}
		envMap := ctx.Previous.(map[string]string)
		envs := make([]bind.ServiceEnvVar, 0, len(envMap))
		for k, v := range envMap {
			envs = append(envs, bind.ServiceEnvVar{
				ServiceName:  args.serviceInstance.ServiceName,
				InstanceName: args.serviceInstance.Name,
				EnvVar: bind.EnvVar{
					Public: false,
					Name:   k,
					Value:  v,
				},
			})
		}
		sort.Slice(envs, func(i, j int) bool {
			return envs[i].Name < envs[j].Name
		})
		addArgs := bind.AddInstanceArgs{
			Envs:          envs,
			ShouldRestart: args.shouldRestart,
			Writer:        args.writer,
		}
		return addArgs, args.app.AddInstance(addArgs)
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		err := args.app.RemoveInstance(bind.RemoveInstanceArgs{
			ServiceName:   args.serviceInstance.ServiceName,
			InstanceName:  args.serviceInstance.Name,
			ShouldRestart: args.shouldRestart,
			Writer:        args.writer,
		})
		if err != nil {
			log.Errorf("[set-bound-envs backward] failed to remove instance: %s", err)
		}
	},
}

var bindUnitsAction = &action.Action{
	Name: "bind-units",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		if args == nil {
			return ctx.Previous, errors.New("invalid arguments for pipeline, expected *bindPipelineArgs.")
		}
		var wg sync.WaitGroup
		si := args.serviceInstance
		units, err := args.app.GetUnits()
		if err != nil {
			return nil, err
		}
		errCh := make(chan error, len(units))
		unboundCh := make(chan bind.Unit, len(units))
		for i := range units {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				unit := units[i]
				err := si.BindUnit(args.app, unit)
				if err == nil {
					unboundCh <- unit
				} else {
					errCh <- err
				}
			}(i)
		}
		wg.Wait()
		close(errCh)
		close(unboundCh)
		if err := <-errCh; err != nil {
			for unit := range unboundCh {
				unbindErr := si.UnbindUnit(args.app, unit)
				if unbindErr != nil {
					log.Errorf("[bind-units forward] failed to unbind unit after error: %s", unbindErr)
				}
			}
			return ctx.Previous, err
		}
		return ctx.Previous, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}

var unbindUnits = action.Action{
	Name: "unbind-units",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindPipelineArgs.")
		}
		var wg sync.WaitGroup
		si := args.serviceInstance
		units, err := args.app.GetUnits()
		if err != nil {
			return nil, err
		}
		errCh := make(chan error, len(units))
		unboundCh := make(chan bind.Unit, len(units))
		for i := range units {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				unit := units[i]
				err := si.UnbindUnit(args.app, unit)
				if err == nil || err == ErrUnitNotBound {
					unboundCh <- unit
				} else {
					errCh <- err
				}
			}(i)
		}
		wg.Wait()
		close(errCh)
		close(unboundCh)
		if err := <-errCh; err != nil {
			for unit := range unboundCh {
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
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		units, err := args.app.GetUnits()
		if err != nil {
			log.Errorf("[unbind-units backward] failed get units to rebind in rollback: %s", err)
		}
		for _, unit := range units {
			err := args.serviceInstance.BindUnit(args.app, unit)
			if err != nil {
				log.Errorf("[unbind-units backward] failed to rebind unit in rollback: %s", err)
			}
		}
	},
	MinParams: 1,
}

var unbindAppDB = action.Action{
	Name: "unbind-app-db",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindPipelineArgs.")
		}
		return nil, args.serviceInstance.updateData(bson.M{"$pull": bson.M{"apps": args.app.GetName()}})
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		err := args.serviceInstance.updateData(bson.M{"$addToSet": bson.M{"apps": args.app.GetName()}})
		if err != nil {
			log.Errorf("[unbind-app-db backward] failed to rebind app in db: %s", err)
		}
	},
	MinParams: 1,
}

var unbindAppEndpoint = action.Action{
	Name: "unbind-app-endpoint",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindPipelineArgs.")
		}
		if endpoint, err := args.serviceInstance.Service().getClient("production"); err == nil {
			err := endpoint.UnbindApp(args.serviceInstance, args.app, args.event, args.requestID)
			if err != nil && err != ErrInstanceNotFoundInAPI {
				return nil, err
			}
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		if endpoint, err := args.serviceInstance.Service().getClient("production"); err == nil {
			_, err := endpoint.BindApp(args.serviceInstance, args.app, args.event, args.requestID)
			if err != nil {
				log.Errorf("[unbind-app-endpoint backward] failed to rebind app in endpoint: %s", err)
			}
		}
	},
	MinParams: 1,
}

var removeBoundEnvs = action.Action{
	Name: "remove-bound-envs",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindPipelineArgs.")
		}
		si := args.serviceInstance
		return nil, args.app.RemoveInstance(bind.RemoveInstanceArgs{
			ServiceName:   si.ServiceName,
			InstanceName:  si.Name,
			ShouldRestart: args.shouldRestart,
			Writer:        args.writer,
		})
	},
	Backward: func(ctx action.BWContext) {
	},
	MinParams: 1,
}
