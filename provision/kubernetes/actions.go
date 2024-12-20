// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"io"

	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router/rebuild"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type updatePipelineParams struct {
	p        *kubernetesProvisioner
	new      *appTypes.App
	old      *appTypes.App
	versions []appTypes.AppVersion
	w        io.Writer
}

var provisionNewApp = action.Action{
	Name: "provision-new-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		return nil, params.p.Provision(ctx.Context, params.new)
	},
	Backward: func(ctx action.BWContext) {
		params := ctx.Params[0].(updatePipelineParams)
		if err := params.p.Destroy(ctx.Context, params.new); err != nil {
			log.Errorf("failed to destroy new app: %v", err)
		}
	},
}

var restartApp = action.Action{
	Name: "restart-new-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		for _, v := range params.versions {
			err := params.p.Restart(ctx.Context, params.new, "", v, params.w)
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		params := ctx.Params[0].(updatePipelineParams)
		if err := backwardCR(ctx.Context, params); err != nil {
			log.Errorf("BACKWARDS failed to update namespace: %v", err)
			return
		}
		err := params.p.Restart(ctx.Context, params.old, "", nil, params.w)
		if err != nil {
			log.Errorf("BACKWARDS error restarting app: %v", err)
		}
	},
}

var rebuildAppRoutes = action.Action{
	Name: "rebuild-routes-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		rebuild.RebuildRoutesWithAppName(params.new.Name, nil)
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		params := ctx.Params[0].(updatePipelineParams)
		if err := backwardCR(ctx.Context, params); err != nil {
			log.Errorf("BACKWARDS failed to update namespace: %v", err)
			return
		}
		rebuild.RebuildRoutesWithAppName(params.old.Name, nil)
	},
}

var destroyOldApp = action.Action{
	Name: "destroy-old-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		err := params.p.Destroy(ctx.Context, params.old)
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
		client, err := clusterForPool(ctx.Context, params.old.Pool)
		if err != nil {
			return nil, err
		}
		return nil, updateAppNamespace(ctx.Context, client, params.old.Name, client.PoolNamespace(params.new.Pool))
	},
	Backward: func(ctx action.BWContext) {
		params := ctx.Params[0].(updatePipelineParams)
		if err := backwardCR(ctx.Context, params); err != nil {
			log.Errorf("BACKWARDS failed to update namespace: %v", err)
		}
	},
}

func backwardCR(ctx context.Context, params updatePipelineParams) error {
	client, err := clusterForPool(ctx, params.old.Pool)
	if err != nil {
		return err
	}
	return updateAppNamespace(ctx, client, params.old.Name, client.PoolNamespace(params.old.Pool))
}

var removeOldAppResources = action.Action{
	Name: "remove-old-app-resources",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		client, err := clusterForPool(ctx.Context, params.old.Pool)
		if err != nil {
			log.Errorf("failed to remove old resources: %v", err)
			return nil, nil
		}
		oldAppCR, err := getAppCR(ctx.Context, client, params.old.Name)
		if err != nil {
			log.Errorf("failed to remove old resources: %v", err)
			return nil, nil
		}
		oldAppCR.Spec.NamespaceName = client.PoolNamespace(params.old.Pool)
		err = params.p.removeResources(ctx.Context, client, oldAppCR, params.old)
		if err != nil {
			log.Errorf("failed to remove old resources: %v", err)
		}
		return nil, nil
	},
}
