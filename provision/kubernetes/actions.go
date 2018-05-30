// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"io"

	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router/rebuild"
)

type updatePipelineParams struct {
	p   *kubernetesProvisioner
	new provision.App
	old provision.App
	w   io.Writer
}

var provisionNewApp = action.Action{
	Name: "provision-new-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		return nil, params.p.Provision(params.new)
	},
	Backward: func(ctx action.BWContext) {
		params := ctx.Params[0].(updatePipelineParams)
		if err := params.p.Destroy(params.new); err != nil {
			log.Errorf("failed to destroy new app: %v", err)
		}
	},
}

var restartApp = action.Action{
	Name: "restart-new-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		return nil, params.p.Restart(params.new, "", params.w)
	},
	Backward: func(ctx action.BWContext) {
		params := ctx.Params[0].(updatePipelineParams)
		if err := backwardCR(params); err != nil {
			log.Errorf("BACKWARDS failed to update namespace: %v", err)
			return
		}
		err := params.p.Restart(params.old, "", params.w)
		if err != nil {
			log.Errorf("BACKWARDS error restarting app: %v", err)
		}
	},
}

var rebuildAppRoutes = action.Action{
	Name: "rebuild-routes-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		rebuild.RoutesRebuildOrEnqueue(params.new.GetName())
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		params := ctx.Params[0].(updatePipelineParams)
		if err := backwardCR(params); err != nil {
			log.Errorf("BACKWARDS failed to update namespace: %v", err)
			return
		}
		rebuild.RoutesRebuildOrEnqueue(params.old.GetName())
	},
}

var destroyOldApp = action.Action{
	Name: "destroy-old-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		err := params.p.Destroy(params.old)
		if err != nil {
			log.Errorf("failed to destroy old app: %v", err)
		}
		return nil, nil
	},
}

var updateAppCR = action.Action{
	Name: "update-app-custom-resource",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		client, err := clusterForPool(params.old.GetPool())
		if err != nil {
			return nil, err
		}
		return nil, updateAppNamespace(client, params.old.GetName(), client.PoolNamespace(params.new.GetPool()))
	},
	Backward: func(ctx action.BWContext) {
		params := ctx.Params[0].(updatePipelineParams)
		if err := backwardCR(params); err != nil {
			log.Errorf("BACKWARDS failed to update namespace: %v", err)
		}
	},
}

func backwardCR(params updatePipelineParams) error {
	client, err := clusterForPool(params.old.GetPool())
	if err != nil {
		return err
	}
	return updateAppNamespace(client, params.old.GetName(), client.PoolNamespace(params.old.GetPool()))
}

var removeOldAppResources = action.Action{
	Name: "remove-old-app-resources",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		client, err := clusterForPool(params.old.GetPool())
		if err != nil {
			log.Errorf("failed to remove old resources: %v", err)
			return nil, nil
		}
		oldAppCR, err := getAppCR(client, params.old.GetName())
		if err != nil {
			log.Errorf("failed to remove old resources: %v", err)
			return nil, nil
		}
		oldAppCR.Spec.NamespaceName = client.PoolNamespace(params.old.GetPool())
		err = params.p.removeResources(client, oldAppCR)
		if err != nil {
			log.Errorf("failed to remove old resources: %v", err)
		}
		return nil, nil
	},
}
