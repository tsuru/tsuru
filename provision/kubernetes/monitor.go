// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router/rebuild"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/informers/internalinterfaces"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
)

const (
	informerSyncTimeout = 10 * time.Second

	leaderElectionName = "tsuru-controller"
)

type clusterController struct {
	mu              sync.Mutex
	cluster         *ClusterClient
	informerFactory informers.SharedInformerFactory
	podInformer     v1informers.PodInformer
	serviceInformer v1informers.ServiceInformer
	nodeInformer    v1informers.NodeInformer
	stopCh          chan struct{}
	cancel          context.CancelFunc
	resourceVers    map[types.NamespacedName]string
	startedAt       time.Time
	leader          int32
	wg              sync.WaitGroup
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
	ctx, cancel := context.WithCancel(context.Background())
	c := &clusterController{
		cluster:      cluster,
		stopCh:       make(chan struct{}),
		cancel:       cancel,
		resourceVers: make(map[types.NamespacedName]string),
		startedAt:    time.Now(),
	}
	err := c.initLeaderElection(ctx)
	if err != nil {
		c.stop()
		return nil, err
	}
	err = c.start()
	if err != nil {
		c.stop()
		return nil, err
	}
	p.clusterControllers[cluster.Name] = c
	return c, nil
}

func stopClusterController(p *kubernetesProvisioner, cluster *ClusterClient) {
	stopClusterControllerByName(p, cluster.Name)
}

func stopClusterControllerByName(p *kubernetesProvisioner, clusterName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clusterControllers[clusterName]; ok {
		c.stop()
	}
	delete(p.clusterControllers, clusterName)
}

func (c *clusterController) stop() {
	close(c.stopCh)
	// HACK(cezarsa): ridiculous hack trying to prevent race condition
	// described in https://github.com/kubernetes/kubernetes/pull/83112. As
	// soon as it's merged we should remove this. Here we wait at least one
	// second between starting and stopping the controller. stop() shouldn't be
	// called too frequently during runtime and it'll be most certainly longer
	// lived, this mostly affects tests.
	<-time.After(time.Second - time.Since(c.startedAt))
	c.cancel()
	c.wg.Wait()
}

func (c *clusterController) isLeader() bool {
	return atomic.LoadInt32(&c.leader) == 1
}

func (c *clusterController) start() error {
	informer, err := c.getPodInformerWait(false)
	if err != nil {
		return err
	}
	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if !c.isLeader() {
				return
			}
			err := c.onAdd(obj)
			if err != nil {
				log.Errorf("[router-update-controller] error on add pod event: %v", err)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if !c.isLeader() {
				return
			}
			err := c.onUpdate(oldObj, newObj)
			if err != nil {
				log.Errorf("[router-update-controller] error on update pod event: %v", err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if !c.isLeader() {
				return
			}
			err := c.onDelete(obj)
			if err != nil {
				log.Errorf("[router-update-controller] error on delete pod event: %v", err)
			}
		},
	})
	return nil
}

func (c *clusterController) onAdd(obj interface{}) error {
	// Pods are never ready on add, ignore and do nothing
	return nil
}

func (c *clusterController) onUpdate(_, newObj interface{}) error {
	newPod := newObj.(*apiv1.Pod)
	name := types.NamespacedName{Namespace: newPod.Namespace, Name: newPod.Name}
	// We keep our own track of handled resource versions and ignore oldObj
	// because of leader election. It's possible for the message containing
	// different resource versions to arrive while the current instance was not
	// a leader yet. We only want to consider a pod version as handled if it
	// arrived while we were leader.
	if c.resourceVers[name] == newPod.ResourceVersion {
		return nil
	}
	c.resourceVers[name] = newPod.ResourceVersion
	c.enqueuePod(newPod)
	return nil
}

func (c *clusterController) onDelete(obj interface{}) error {
	if pod, ok := obj.(*apiv1.Pod); ok {
		c.enqueuePodDelete(pod)
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
	c.enqueuePodDelete(pod)
	return nil
}

func (c *clusterController) enqueuePodDelete(pod *apiv1.Pod) {
	name := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}
	delete(c.resourceVers, name)
	c.enqueuePod(pod)
}

func (c *clusterController) enqueuePod(pod *apiv1.Pod) {
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

func (c *clusterController) getPodInformer() (v1informers.PodInformer, error) {
	return c.getPodInformerWait(true)
}

func (c *clusterController) getServiceInformer() (v1informers.ServiceInformer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.serviceInformer == nil {
		err := c.withInformerFactory(func(factory informers.SharedInformerFactory) {
			c.serviceInformer = factory.Core().V1().Services()
			c.serviceInformer.Informer()
		})
		if err != nil {
			return nil, err
		}
	}
	err := c.waitForSync(c.serviceInformer.Informer())
	return c.serviceInformer, err
}

func (c *clusterController) getNodeInformer() (v1informers.NodeInformer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nodeInformer == nil {
		err := c.withInformerFactory(func(factory informers.SharedInformerFactory) {
			c.nodeInformer = factory.Core().V1().Nodes()
			c.nodeInformer.Informer()
		})
		if err != nil {
			return nil, err
		}
	}
	err := c.waitForSync(c.nodeInformer.Informer())
	return c.nodeInformer, err
}

func (c *clusterController) getPodInformerWait(wait bool) (v1informers.PodInformer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.podInformer == nil {
		err := c.withInformerFactory(func(factory informers.SharedInformerFactory) {
			c.podInformer = factory.Core().V1().Pods()
			c.podInformer.Informer()
		})
		if err != nil {
			return nil, err
		}
	}
	var err error
	if wait {
		err = c.waitForSync(c.podInformer.Informer())
	}
	return c.podInformer, err
}

func (c *clusterController) withInformerFactory(fn func(factory informers.SharedInformerFactory)) error {
	factory, err := c.getFactory()
	if err != nil {
		return err
	}
	fn(factory)
	factory.Start(c.stopCh)
	return nil
}

func (c *clusterController) getFactory() (informers.SharedInformerFactory, error) {
	if c.informerFactory != nil {
		return c.informerFactory, nil
	}
	var err error
	c.informerFactory, err = InformerFactory(c.cluster)
	return c.informerFactory, err
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

func (c *clusterController) waitForSync(informer cache.SharedInformer) error {
	if informer.HasSynced() {
		return nil
	}
	ctx, cancel := contextWithCancelByChannel(context.Background(), c.stopCh, informerSyncTimeout)
	defer cancel()
	cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
	return errors.Wrap(ctx.Err(), "error waiting for informer sync")
}

func (c *clusterController) initLeaderElection(ctx context.Context) error {
	id, err := os.Hostname()
	if err != nil {
		return err
	}
	broadcaster := record.NewBroadcaster()
	recorder := broadcaster.NewRecorder(scheme.Scheme, apiv1.EventSource{
		Component: leaderElectionName,
	})
	// err = ensureNamespace(c.cluster, c.cluster.Namespace())
	// if err != nil {
	// 	return err
	// }
	lock, err := resourcelock.New(
		resourcelock.EndpointsResourceLock,
		c.cluster.Namespace(),
		leaderElectionName,
		c.cluster.CoreV1(),
		resourcelock.ResourceLockConfig{
			Identity:      id,
			EventRecorder: recorder,
		},
	)
	if err != nil {
		return err
	}
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				atomic.StoreInt32(&c.leader, 1)
			},
			OnStoppedLeading: func() {
				atomic.StoreInt32(&c.leader, 0)
			},
		},
	})
	if err != nil {
		return err
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			le.Run(ctx)
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	return nil
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
		ls := provision.IsServiceLabelSet(tsuruLabelPrefix)
		opts.LabelSelector = labels.SelectorFromSet(labels.Set(ls.ToIsServiceSelector())).String()
	})
	return informers.NewFilteredSharedInformerFactory(cli, time.Minute, metav1.NamespaceAll, tweakFunc), nil
}
