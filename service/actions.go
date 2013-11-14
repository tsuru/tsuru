// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"
	"github.com/globocom/tsuru/action"
)

// createServiceInstance in an action that calls the service endpoint
// to creates the service instance.
//
// The first argument in the context must be an Service.
// The second argument in the context must be an ServiceInstance.
var createServiceInstance = action.Action{
	Name: "create-service-instance",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var (
			service  Service
			instance ServiceInstance
		)
		switch ctx.Params[0].(type) {
		case Service:
			service = ctx.Params[0].(Service)
		default:
			return nil, errors.New("First parameter must be a Service.")
		}
		switch ctx.Params[1].(type) {
		case ServiceInstance:
			instance = ctx.Params[1].(ServiceInstance)
		default:
			return nil, errors.New("Second parameter must be a ServiceInstance.")
		}
		endpoint, err := service.getClient("production")
		if err != nil {
			return nil, err
		}
		err = endpoint.Create(&instance)
		if err != nil {
			return nil, err
		}
		return instance, nil
	},
	Backward: func(ctx action.BWContext) {
	},
	MinParams: 2,
}
