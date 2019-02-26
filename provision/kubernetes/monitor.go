// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router/rebuild"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/informers/internalinterfaces"
	"k8s.io/client-go/tools/cache"
)

const (
	informerSyncTimeout = time.Minute
)

type clusterController struct {
	mu              sync.Mutex
	cluster         *ClusterClient
	informerFactory informers.SharedInformerFactory
	podInformer     v1informers.PodInformer
	serviceInformer v1informers.ServiceInformer
	nodeInformer    v1informers.NodeInformer
	stopCh          chan struct{}
}

func initAllControllers(p *kubernetesProvisioner) error {
	return forEachCluster(func(client *ClusterClient) error {
		_, err := getClusterController(p, client)
		return err
	})
}

func getClusterController(p *kubernetesProvisioner, cluster *ClusterClient) (*clusterController, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clusterControllers[cluster.Name]; ok {
		return c, nil
	}
	c := &clusterController{
		cluster: cluster,
		stopCh:  make(chan struct{}),
	}
	err := c.start()
	if err != nil {
		return nil, err
	}
	p.clusterControllers[cluster.Name] = c
	return c, nil
}

func stopClusterController(p *kubernetesProvisioner, cluster *ClusterClient) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clusterControllers[cluster.Name]; ok {
		c.stop()
	}
	delete(p.clusterControllers, cluster.Name)
}

func (c *clusterController) stop() {
	close(c.stopCh)
}

func (c *clusterController) start() error {
	informer, err := c.getPodInformerWait(false)
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

func (m *clusterController) onAdd(obj interface{}) error {
	// Pods are never ready on add, ignore and do nothing
	return nil
}

func (c *clusterController) onUpdate(oldObj, newObj interface{}) error {
	newPod := oldObj.(*apiv1.Pod)
	oldPod := newObj.(*apiv1.Pod)
	if newPod.ResourceVersion == oldPod.ResourceVersion {
		return nil
	}
	c.addPod(newPod)
	return nil
}

func (c *clusterController) onDelete(obj interface{}) error {
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

func (c *clusterController) addPod(pod *apiv1.Pod) {
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

func (p *clusterController) getPodInformer() (v1informers.PodInformer, error) {
	return p.getPodInformerWait(true)
}

func (p *clusterController) getServiceInformer() (v1informers.ServiceInformer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.serviceInformer == nil {
		err := p.withInformerFactory(func(factory informers.SharedInformerFactory) {
			p.serviceInformer = factory.Core().V1().Services()
			p.serviceInformer.Informer()
		})
		if err != nil {
			return nil, err
		}
	}
	err := p.waitForSync(p.serviceInformer.Informer())
	return p.serviceInformer, err
}

func (p *clusterController) getNodeInformer() (v1informers.NodeInformer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.nodeInformer == nil {
		err := p.withInformerFactory(func(factory informers.SharedInformerFactory) {
			p.nodeInformer = factory.Core().V1().Nodes()
			p.nodeInformer.Informer()
		})
		if err != nil {
			return nil, err
		}
	}
	err := p.waitForSync(p.nodeInformer.Informer())
	return p.nodeInformer, err
}

func (p *clusterController) getPodInformerWait(wait bool) (v1informers.PodInformer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.podInformer == nil {
		err := p.withInformerFactory(func(factory informers.SharedInformerFactory) {
			p.podInformer = factory.Core().V1().Pods()
			p.podInformer.Informer()
		})
		if err != nil {
			return nil, err
		}
	}
	var err error
	if wait {
		err = p.waitForSync(p.podInformer.Informer())
	}
	return p.podInformer, err
}

func (p *clusterController) withInformerFactory(fn func(factory informers.SharedInformerFactory)) error {
	factory, err := p.getFactory()
	if err != nil {
		return err
	}
	fn(factory)
	factory.Start(p.stopCh)
	return nil
}

func (p *clusterController) getFactory() (informers.SharedInformerFactory, error) {
	if p.informerFactory != nil {
		return p.informerFactory, nil
	}
	var err error
	p.informerFactory, err = InformerFactory(p.cluster)
	return p.informerFactory, err
}

func contextWithCancelByChannel(ctx context.Context, ch chan struct{}, timeout time.Duration) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	go func() {
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
			return
		}
	}()
	return ctx, cancel
}

func (p *clusterController) waitForSync(informer cache.SharedInformer) error {
	if informer.HasSynced() {
		return nil
	}
	ctx, cancel := contextWithCancelByChannel(context.Background(), p.stopCh, informerSyncTimeout)
	defer cancel()
	cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
	return ctx.Err()
}

var InformerFactory = func(client *ClusterClient) (informers.SharedInformerFactory, error) {
	timeout := client.restConfig.Timeout
	restConfig := *client.restConfig
	restConfig.Timeout = 0
	cli, err := ClientForConfig(&restConfig)
	if err != nil {
		return nil, err
	}
	tweakFunc := internalinterfaces.TweakListOptionsFunc(func(opts *metav1.ListOptions) {
		if opts.TimeoutSeconds == nil {
			timeoutSec := int64(timeout.Seconds())
			opts.TimeoutSeconds = &timeoutSec
		}
	})
	return informers.NewFilteredSharedInformerFactory(cli, time.Minute, metav1.NamespaceAll, tweakFunc), nil
}
