// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"fmt"
	"io"
	"sort"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/servicemanager"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
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
		instance, ok := ctx.Params[1].(*ServiceInstance)
		if !ok {
			return nil, errors.New("Second parameter must be a *ServiceInstance.")
		}
		endpoint, err := service.getClientForPool(ctx.Context, instance.Pool)
		if err != nil {
			return nil, err
		}
		evt, ok := ctx.Params[2].(*event.Event)
		if !ok {
			return nil, errors.New("Third parameter must be an event.")
		}
		requestID, ok := ctx.Params[3].(string)
		if !ok {
			return nil, errors.New("RequestID should be a string.")
		}
		err = endpoint.Create(ctx.Context, instance, evt, requestID)
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
		instance, ok := ctx.Params[1].(*ServiceInstance)
		if !ok {
			return
		}
		endpoint, err := service.getClientForPool(ctx.Context, instance.Pool)
		if err != nil {
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
		endpoint.Destroy(ctx.Context, instance, evt, requestID)
	},
	MinParams: 3,
}

// createServiceInstance is an action that inserts an instance in the database.
//
// The second argument in the context must be a Service Instance.
var createServiceInstance = action.Action{
	Name: "create-service-instance",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		instance, ok := ctx.Params[1].(*ServiceInstance)
		if !ok {
			return nil, errors.New("Second parameter must be a *ServiceInstance.")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return nil, conn.ServiceInstances().Insert(instance)
	},
	Backward: func(ctx action.BWContext) {
		instance, ok := ctx.Params[1].(*ServiceInstance)
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
					"plan_name":   updateData.PlanName,
					"parameters":  updateData.Parameters,
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
					"plan_name":   instance.PlanName,
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
		instance, ok := ctx.Params[1].(ServiceInstance)
		if !ok {
			return nil, errors.New("Second parameter must be a ServiceInstance.")
		}
		endpoint, err := service.getClientForPool(ctx.Context, instance.Pool)
		if err != nil {
			return nil, err
		}
		evt, ok := ctx.Params[3].(*event.Event)
		if !ok {
			return nil, errors.New("Third parameter must be an event.")
		}
		requestID, ok := ctx.Params[4].(string)
		if !ok {
			return nil, errors.New("RequestID should be a string.")
		}
		err = endpoint.Update(ctx.Context, &instance, evt, requestID)
		if err != nil {
			return nil, err
		}
		return instance, nil
	},
	Backward:  func(ctx action.BWContext) {},
	MinParams: 4,
}

type bindAppPipelineArgs struct {
	app             bind.App
	writer          io.Writer
	serviceInstance *ServiceInstance
	params          BindAppParameters
	event           *event.Event
	requestID       string
	shouldRestart   bool
	forceRemove     bool
}

type bindJobPipelineArgs struct {
	job             *jobTypes.Job
	writer          io.Writer
	serviceInstance *ServiceInstance
	event           *event.Event
	requestID       string
	forceRemove     bool
}

var bindAppDBAction = &action.Action{
	Name: "bind-app-db",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindAppPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindAppPipelineArgs.")
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
		args, _ := ctx.Params[0].(*bindAppPipelineArgs)
		if err := args.serviceInstance.updateData(bson.M{"$pull": bson.M{"apps": args.app.GetName()}}); err != nil {
			log.Errorf("[bind-app-db backward] could not remove app from service instance: %s", err)
		}
	},
	MinParams: 1,
}

var bindJobDBAction = &action.Action{
	Name: "bind-job-db",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindJobPipelineArgs.")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		si := args.serviceInstance
		updateOp := bson.M{"$addToSet": bson.M{"jobs": args.job.Name}}
		err = conn.ServiceInstances().Update(bson.M{"name": si.Name, "service_name": si.ServiceName, "jobs": bson.M{"$ne": args.job.Name}}, updateOp)
		if err != nil {
			if err == mgo.ErrNotFound {
				return nil, ErrJobAlreadyBound
			}
			return nil, err
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)
		if err := args.serviceInstance.updateData(bson.M{"$pull": bson.M{"jobs": args.job.Name}}); err != nil {
			log.Errorf("[bind-job-db backward] could not remove job from service instance: %s", err)
		}
	},
	MinParams: 1,
}

var bindAppEndpointAction = &action.Action{
	Name: "bind-app-endpoint",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindAppPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindAppPipelineArgs.")
		}
		s, err := Get(ctx.Context, args.serviceInstance.ServiceName)
		if err != nil {
			return nil, err
		}
		endpoint, err := s.getClientForPool(ctx.Context, args.serviceInstance.Pool)
		if err != nil {
			return nil, err
		}
		return endpoint.BindApp(ctx.Context, args.serviceInstance, args.app, args.params, args.event, args.requestID)
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindAppPipelineArgs)
		s, err := Get(ctx.Context, args.serviceInstance.ServiceName)
		if err != nil {
			log.Errorf("[bind-app-endpoint backward] could not service from instance: %s", err)
			return
		}
		endpoint, err := s.getClientForPool(ctx.Context, args.serviceInstance.Pool)
		if err != nil {
			log.Errorf("[bind-app-endpoint backward] could not get endpoint: %s", err)
			return
		}
		err = endpoint.UnbindApp(ctx.Context, args.serviceInstance, args.app, args.event, args.requestID)
		if err != nil {
			log.Errorf("[bind-app-endpoint backward] failed to unbind unit: %s", err)
		}
	},
	MinParams: 1,
}

var bindJobEndpointAction = &action.Action{
	Name: "bind-job-endpoint",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindJobPipelineArgs.")
		}
		s, err := Get(ctx.Context, args.serviceInstance.ServiceName)
		if err != nil {
			return nil, err
		}
		endpoint, err := s.getClientForPool(ctx.Context, args.serviceInstance.Pool)
		if err != nil {
			return nil, err
		}
		return endpoint.BindJob(ctx.Context, args.serviceInstance, args.job, args.event, args.requestID)
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)
		s, err := Get(ctx.Context, args.serviceInstance.ServiceName)
		if err != nil {
			log.Errorf("[bind-job-endpoint backward] could not service from instance: %s", err)
			return
		}
		endpoint, err := s.getClientForPool(ctx.Context, args.serviceInstance.Pool)
		if err != nil {
			log.Errorf("[bind-job-endpoint backward] could not get endpoint: %s", err)
			return
		}
		err = endpoint.UnbindJob(ctx.Context, args.serviceInstance, args.job, args.event, args.requestID)
		if err != nil {
			log.Errorf("[bind-job-endpoint backward] failed to unbind unit: %s", err)
		}
	},
	MinParams: 1,
}

var setBoundEnvsAction = &action.Action{
	Name: "set-bound-envs",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindAppPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindAppPipelineArgs.")
		}
		envMap := ctx.Previous.(map[string]string)
		envs := make([]bindTypes.ServiceEnvVar, 0, len(envMap))
		for k, v := range envMap {
			envs = append(envs, bindTypes.ServiceEnvVar{
				ServiceName:  args.serviceInstance.ServiceName,
				InstanceName: args.serviceInstance.Name,
				EnvVar: bindTypes.EnvVar{
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
		args, _ := ctx.Params[0].(*bindAppPipelineArgs)
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

var setJobBoundEnvsAction = &action.Action{
	Name: "set-job-bound-envs",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindJobPipelineArgs.")
		}
		envMap := ctx.Previous.(map[string]string)
		envs := make([]bindTypes.ServiceEnvVar, 0, len(envMap))
		for k, v := range envMap {
			envs = append(envs, bindTypes.ServiceEnvVar{
				ServiceName:  args.serviceInstance.ServiceName,
				InstanceName: args.serviceInstance.Name,
				EnvVar: bindTypes.EnvVar{
					Public: false,
					Name:   k,
					Value:  v,
				},
			})
		}
		sort.Slice(envs, func(i, j int) bool {
			return envs[i].Name < envs[j].Name
		})

		addArgs := jobTypes.AddInstanceArgs{
			Envs:   envs,
			Writer: args.writer,
		}
		return addArgs, servicemanager.Job.AddServiceEnv(ctx.Context, args.job, addArgs)
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)

		err := servicemanager.Job.RemoveServiceEnv(ctx.Context, args.job, jobTypes.RemoveInstanceArgs{
			ServiceName:  args.serviceInstance.ServiceName,
			InstanceName: args.serviceInstance.Name,
			Writer:       args.writer,
		})
		if err != nil {
			log.Errorf("[set-bound-envs backward] failed to remove instance: %s", err)
		}
	},
}

var reloadJobProvisioner = &action.Action{
	Name: "reload-job-provisioner",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindJobPipelineArgs.")
		}

		job, err := servicemanager.Job.GetByName(ctx.Context, args.job.Name)
		if err != nil {
			return nil, err
		}

		return nil, servicemanager.Job.UpdateJobProv(ctx.Context, job)
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)

		err := servicemanager.Job.UpdateJobProv(ctx.Context, args.job)
		if err != nil {
			log.Errorf("[reload-job-provisioner backward] failed to update provisioner with old job: %s", err)
		}
	},
}

var unbindAppDB = action.Action{
	Name: "unbind-app-db",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindAppPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindAppPipelineArgs.")
		}
		return nil, args.serviceInstance.updateData(bson.M{"$pull": bson.M{"apps": args.app.GetName()}})
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindAppPipelineArgs)
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
		args, _ := ctx.Params[0].(*bindAppPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindAppPipelineArgs.")
		}
		s, err := Get(ctx.Context, args.serviceInstance.ServiceName)
		if err != nil {
			return nil, err
		}
		if endpoint, err := s.getClientForPool(ctx.Context, args.serviceInstance.Pool); err == nil {
			err := endpoint.UnbindApp(ctx.Context, args.serviceInstance, args.app, args.event, args.requestID)
			if err != nil && err != ErrInstanceNotFoundInAPI {
				if args.forceRemove {
					msg := fmt.Sprintf("[unbind-app-endpoint] ignored error due to force: %v", err.Error())
					if args.writer != nil {
						fmt.Fprintln(args.writer, msg)
					}
					log.Errorf("%s", msg)
					return nil, nil
				}
				return nil, err
			}
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindAppPipelineArgs)
		s, err := Get(ctx.Context, args.serviceInstance.ServiceName)
		if err != nil {
			log.Errorf("[unbind-app-endpoint backward] failed to rebind app in endpoint: %s", err)
			return
		}
		if endpoint, err := s.getClientForPool(ctx.Context, args.serviceInstance.Pool); err == nil {
			_, err := endpoint.BindApp(ctx.Context, args.serviceInstance, args.app, args.params, args.event, args.requestID)
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
		args, _ := ctx.Params[0].(*bindAppPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindAppPipelineArgs.")
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

var unbindJobDB = action.Action{
	Name: "unbind-job-db",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindJobPipelineArgs.")
		}
		return nil, args.serviceInstance.updateData(bson.M{"$pull": bson.M{"jobs": args.job.Name}})
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)
		err := args.serviceInstance.updateData(bson.M{"$addToSet": bson.M{"jobs": args.job.Name}})
		if err != nil {
			log.Errorf("[unbind-job-db backward] failed to rebind job in db: %s", err)
		}
	},
	MinParams: 1,
}

var unbindJobEndpoint = action.Action{
	Name: "unbind-job-endpoint",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindJobPipelineArgs.")
		}
		s, err := Get(ctx.Context, args.serviceInstance.ServiceName)
		if err != nil {
			return nil, err
		}
		if endpoint, err := s.getClientForPool(ctx.Context, args.serviceInstance.Pool); err == nil {
			err := endpoint.UnbindJob(ctx.Context, args.serviceInstance, args.job, args.event, args.requestID)
			if err != nil && err != ErrInstanceNotFoundInAPI {
				if args.forceRemove {
					msg := fmt.Sprintf("[unbind-job-endpoint] ignored error due to force: %v", err.Error())
					if args.writer != nil {
						fmt.Fprintln(args.writer, msg)
					}
					log.Errorf("%s", msg)
					return nil, nil
				}
				return nil, err
			}
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)
		s, err := Get(ctx.Context, args.serviceInstance.ServiceName)
		if err != nil {
			log.Errorf("[unbind-job-endpoint backward] failed to rebind job in endpoint: %s", err)
			return
		}
		if endpoint, err := s.getClientForPool(ctx.Context, args.serviceInstance.Pool); err == nil {
			_, err := endpoint.BindJob(ctx.Context, args.serviceInstance, args.job, args.event, args.requestID)
			if err != nil {
				log.Errorf("[unbind-job-endpoint backward] failed to rebind job in endpoint: %s", err)
			}
		}
	},
	MinParams: 1,
}

var removeJobBoundEnvs = action.Action{
	Name: "remove-job-bound-envs",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args, _ := ctx.Params[0].(*bindJobPipelineArgs)
		if args == nil {
			return nil, errors.New("invalid arguments for pipeline, expected *bindJobPipelineArgs.")
		}

		rmInstanceArgs := jobTypes.RemoveInstanceArgs{
			ServiceName:  args.serviceInstance.ServiceName,
			InstanceName: args.serviceInstance.Name,
			Writer:       args.writer,
		}
		return nil, servicemanager.Job.RemoveServiceEnv(ctx.Context, args.job, rmInstanceArgs)
	},
	Backward: func(ctx action.BWContext) {
	},
	MinParams: 1,
}
