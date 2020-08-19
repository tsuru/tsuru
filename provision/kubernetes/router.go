// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"errors"

	"github.com/tsuru/tsuru/provision/kubernetes/routers"
	appTypes "github.com/tsuru/tsuru/types/app"
)

const (
	swapLabel = "tsuru.io/swapped-with"
)

var RouterTypeLoadbalancer = "loadbalancer"

func (p *kubernetesProvisioner) EnsureRouter(app appTypes.App, routerType string, opts map[string]string) error {
	kubeRouter, err := getKubeRouter(app, routerType)
	if err != nil {
		return err
	}
	return kubeRouter.EnsureRouter(app, opts)

	// lbService, err := getLBService(clusterClient, ns, app.GetName())
	// if err != nil {
	//	if !k8sErrors.IsNotFound(err) {
	//		return err
	//	}
	//	lbService = &v1.Service{
	//		ObjectMeta: metav1.ObjectMeta{
	//			Name:      serviceName(app.GetName()),
	//			Namespace: ns,
	//		},
	//		Spec: v1.ServiceSpec{
	//			Type: v1.ServiceTypeLoadBalancer,
	//		},
	//	}
	// }
	// if _, isSwapped := isSwapped(lbService.ObjectMeta); isSwapped {
	//	return nil
	// }
	// fmt.Println("Ensuring the app", lbService)
	// if err != nil {
	//	return err
	// }

	return nil
}

func getKubeRouter(app appTypes.App, routerType string) (routers.KubeRouter, error) {
	clusterClient, err := clusterForPool(app.GetPool())
	if err != nil {
		return nil, err
	}

	ns, err := clusterClient.AppNamespace(app)
	if err != nil {
		return nil, err
	}

	if routerType == RouterTypeLoadbalancer {
		return &routers.LBService{
			BaseService: routers.BaseService{
				Namespace: ns,
				Client:    clusterClient.Interface,
			},
		}, nil
	}

	return nil, errors.New("no valid routerType")
}
