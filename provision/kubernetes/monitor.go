// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router/rebuild"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

var (
	clusterControllers   = map[string]*routerController{}
	clusterControllersMu sync.Mutex
)

type routerController struct {
	p       *kubernetesProvisioner
	cluster *ClusterClient
}

func initAllControllers(p *kubernetesProvisioner) error {
	return forEachCluster(func(client *ClusterClient) error {
		_, err := newRouterController(p, client)
		return err
	})
}

func newRouterController(p *kubernetesProvisioner, cluster *ClusterClient) (*routerController, error) {
	clusterControllersMu.Lock()
	defer clusterControllersMu.Unlock()
	if c, ok := clusterControllers[cluster.Name]; ok {
		return c, nil
	}
	c := &routerController{
		p:       p,
		cluster: cluster,
	}
	err := c.start()
	if err != nil {
		return nil, err
	}
	clusterControllers[cluster.Name] = c
	return c, nil
}

func (c *routerController) start() error {
	informer, err := c.p.podInformerForCluster(c.cluster)
	if err != nil {
		return err
	}
	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			err := c.onAdd(obj)
			if err != nil {
				log.Errorf("[router-update-controller] error on add pod event: %v", err)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			err := c.onUpdate(oldObj, newObj)
			if err != nil {
				log.Errorf("[router-update-controller] error on update pod event: %v", err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			err := c.onDelete(obj)
			if err != nil {
				log.Errorf("[router-update-controller] error on delete pod event: %v", err)
			}
		},
	})
	return nil
}

func (m *routerController) onAdd(obj interface{}) error {
	// Pods are never ready on add, ignore and do nothing
	return nil
}

func (c *routerController) onUpdate(oldObj, newObj interface{}) error {
	newPod := oldObj.(*apiv1.Pod)
	oldPod := newObj.(*apiv1.Pod)
	if newPod.ResourceVersion == oldPod.ResourceVersion {
		return nil
	}
	c.addPod(newPod)
	return nil
}

func (c *routerController) onDelete(obj interface{}) error {
	if pod, ok := obj.(*apiv1.Pod); ok {
		c.addPod(pod)
		return nil
	}
	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		return errors.Errorf("couldn't get object from tombstone %#v", obj)
	}
	pod, ok := tombstone.Obj.(*apiv1.Pod)
	if !ok {
		return errors.Errorf("tombstone contained object that is not a Pod: %#v", obj)
	}
	c.addPod(pod)
	return nil
}

func (c *routerController) addPod(pod *apiv1.Pod) {
	labelSet := labelSetFromMeta(&pod.ObjectMeta)
	appName := labelSet.AppName()
	if appName == "" {
		return
	}
	if labelSet.IsDeploy() || labelSet.IsIsolatedRun() {
		return
	}
	routerLocal, _ := c.cluster.RouterAddressLocal(labelSet.AppPool())
	if routerLocal {
		rebuild.EnqueueRoutesRebuild(appName)
	}
}
