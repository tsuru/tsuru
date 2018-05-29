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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		rebuild.RoutesRebuildOrEnqueue(params.old.GetName())
	},
}

var destroyOldApp = action.Action{
	Name: "destroy-old-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		return nil, params.p.Destroy(params.old)
	},
	Backward: func(ctx action.BWContext) {
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
		tclient, err := TsuruClientForConfig(client.restConfig)
		if err != nil {
			return nil, err
		}
		oldAppCR, err := tclient.TsuruV1().Apps(client.Namespace()).Get(params.old.GetName(), metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		oldAppCR.Spec.NamespaceName = client.PoolNamespace(params.new.GetPool())
		_, err = tclient.TsuruV1().Apps(client.Namespace()).Update(oldAppCR)
		return nil, err
	},
	Backward: func(ctx action.BWContext) {
		params := ctx.Params[0].(updatePipelineParams)
		client, err := clusterForPool(params.old.GetPool())
		if err != nil {
			log.Errorf("failed to get client for pool: %v", err)
			return
		}
		tclient, err := TsuruClientForConfig(client.restConfig)
		if err != nil {
			log.Errorf("failed to get tclient for pool: %v", err)
			return
		}
		oldAppCR, err := tclient.TsuruV1().Apps(client.Namespace()).Get(params.old.GetName(), metav1.GetOptions{})
		if err != nil {
			log.Errorf("failed to get cr for app: %v", err)
			return
		}
		oldAppCR.Spec.NamespaceName = client.PoolNamespace(params.old.GetPool())
		_, err = tclient.TsuruV1().Apps(client.Namespace()).Update(oldAppCR)
		if err != nil {
			log.Errorf("failed to update app cr: %v", err)
		}
	},
}

var removeOldAppResources = action.Action{
	Name: "remove-old-app-resources",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		params := ctx.Params[0].(updatePipelineParams)
		client, err := clusterForPool(params.old.GetPool())
		if err != nil {
			return nil, err
		}
		tclient, err := TsuruClientForConfig(client.restConfig)
		if err != nil {
			return nil, err
		}
		oldAppCR, err := tclient.TsuruV1().Apps(client.Namespace()).Get(params.old.GetName(), metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		oldAppCR.Spec.NamespaceName = client.PoolNamespace(params.old.GetPool())
		return nil, params.p.removeResources(client, oldAppCR)
	},
	Backward: func(ctx action.BWContext) {
	},
}
