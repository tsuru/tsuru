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
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router/rebuild"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	vpaInformers "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions"
	vpaV1Informers "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions/autoscaling.k8s.io/v1"
	vpaInternalInterfaces "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions/internalinterfaces"
	"k8s.io/client-go/informers"
	autoscalingInformers "k8s.io/client-go/informers/autoscaling/v2beta2"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/informers/internalinterfaces"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
)

const (
	informerSyncTimeout = 10 * time.Second

	leaderElectionName = "tsuru-controller"
)

var eventKindsIgnoreRebuild = []string{
	permission.PermAppDeploy.FullName(),
	permission.PermAppUpdateUnitAdd.FullName(),
	permission.PermAppUpdateUnitRemove.FullName(),
	permission.PermAppUpdateRestart.FullName(),
	permission.PermAppUpdateStop.FullName(),
	permission.PermAppUpdateStart.FullName(),
	permission.PermAppUpdateRoutable.FullName(),
}

type podListener interface {
	OnPodEvent(*apiv1.Pod)
}
type podListeners map[string]podListener

type clusterController struct {
	mu                      sync.Mutex
	cluster                 *ClusterClient
	informerFactory         informers.SharedInformerFactory
	filteredInformerFactory informers.SharedInformerFactory
	vpaInformerFactory      vpaInformers.SharedInformerFactory
	podInformer             v1informers.PodInformer
	serviceInformer         v1informers.ServiceInformer
	nodeInformer            v1informers.NodeInformer
	hpaInformer             autoscalingInformers.HorizontalPodAutoscalerInformer
	vpaInformer             vpaV1Informers.VerticalPodAutoscalerInformer
	stopCh                  chan struct{}
	cancel                  context.CancelFunc
	resourceReadyCache      map[types.NamespacedName]bool
	startedAt               time.Time
	podListeners            map[string]podListeners
	podListenersMu          sync.RWMutex
	wg                      sync.WaitGroup
	leader                  int32
}

func initAllControllers(p *kubernetesProvisioner) error {
	return forEachCluster(context.Background(), func(client *ClusterClient) error {
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
		cluster:            cluster,
		stopCh:             make(chan struct{}),
		cancel:             cancel,
		resourceReadyCache: make(map[types.NamespacedName]bool),
		startedAt:          time.Now(),
		podListeners:       make(map[string]podListeners),
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

	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod, ok := obj.(*apiv1.Pod)
			if !ok {
				return
			}

			c.notifyPodChanges(pod)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			newPod, ok := newObj.(*apiv1.Pod)
			if !ok {
				return
			}
			c.notifyPodChanges(newPod)
		},
	})

	return nil
}

func (c *clusterController) onAdd(obj interface{}) error {
	// Pods are never ready on add, ignore and do nothing
	return nil
}

func (c *clusterController) onUpdate(_, newObj interface{}) error {
	newPod, ok := newObj.(*apiv1.Pod)
	if !ok {
		return errors.Errorf("object is not a pod: %#v", newObj)
	}
	name := types.NamespacedName{Namespace: newPod.Namespace, Name: newPod.Name}
	podReady := isPodReady(newPod)
	// We keep track of the last seen ready state and only update the routes if
	// it changes.
	if c.resourceReadyCache[name] == podReady {
		return nil
	}
	c.resourceReadyCache[name] = podReady
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

func (c *clusterController) notifyPodChanges(pod *apiv1.Pod) {
	c.podListenersMu.RLock()
	defer c.podListenersMu.RUnlock()

	appName := pod.ObjectMeta.Labels[tsuruLabelAppName]
	listeners, contains := c.podListeners[appName]
	if !contains {
		return
	}

	for _, listener := range listeners {
		listener.OnPodEvent(pod)
	}
}

func (c *clusterController) addPodListener(appName string, key string, listener podListener) {
	c.podListenersMu.Lock()
	defer c.podListenersMu.Unlock()

	if c.podListeners[appName] == nil {
		c.podListeners[appName] = make(podListeners)
	}
	c.podListeners[appName][key] = listener
}

func (c *clusterController) removePodListener(appName string, key string) {
	c.podListenersMu.Lock()
	defer c.podListenersMu.Unlock()

	if c.podListeners[appName] != nil {
		delete(c.podListeners[appName], key)
	}
}

func (c *clusterController) enqueuePodDelete(pod *apiv1.Pod) {
	name := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}
	delete(c.resourceReadyCache, name)
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
		var runningTrue bool = true
		evts, err := event.List(&event.Filter{
			Running: &runningTrue,
			Target: event.Target{
				Type:  event.TargetTypeApp,
				Value: appName,
			},
			KindType:  event.KindTypePermission,
			KindNames: eventKindsIgnoreRebuild,
			Limit:     1,
		})
		if err == nil && len(evts) > 0 {
			return
		}
		runRoutesRebuild(appName)
	}
}

// runRoutesRebuild is used in tests for mocking rebuild
var runRoutesRebuild = func(appName string) {
	rebuild.EnqueueRoutesRebuild(appName)
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

func (c *clusterController) getHPAInformer() (autoscalingInformers.HorizontalPodAutoscalerInformer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hpaInformer == nil {
		err := c.withFilteredInformerFactory(func(factory informers.SharedInformerFactory) {
			c.hpaInformer = factory.Autoscaling().V2beta2().HorizontalPodAutoscalers()
			c.hpaInformer.Informer()
		})
		if err != nil {
			return nil, err
		}
	}
	err := c.waitForSync(c.hpaInformer.Informer())
	return c.hpaInformer, err
}

func (c *clusterController) getVPAInformer() (vpaV1Informers.VerticalPodAutoscalerInformer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vpaInformer == nil {
		if c.vpaInformerFactory == nil {
			args := newInformerFactoryArgs(c.cluster, tsuruOnlyTweakFunc())
			cli, err := VPAClientForConfig(args.restConfig)
			if err != nil {
				return nil, err
			}
			tweak := vpaInternalInterfaces.TweakListOptionsFunc(args.tweak)
			c.vpaInformerFactory = vpaInformers.NewFilteredSharedInformerFactory(cli, args.resync, metav1.NamespaceAll, tweak)
		}
		c.vpaInformer = c.vpaInformerFactory.Autoscaling().V1().VerticalPodAutoscalers()
		c.vpaInformer.Informer()
		c.vpaInformerFactory.Start(c.stopCh)
	}
	err := c.waitForSync(c.vpaInformer.Informer())
	return c.vpaInformer, err
}

func (c *clusterController) getPodInformerWait(wait bool) (v1informers.PodInformer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.podInformer == nil {
		err := c.withFilteredInformerFactory(func(factory informers.SharedInformerFactory) {
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
	c.informerFactory, err = InformerFactory(c.cluster, nil)
	return c.informerFactory, err
}

func (c *clusterController) withFilteredInformerFactory(fn func(factory informers.SharedInformerFactory)) error {
	factory, err := c.getFilteredFactory()
	if err != nil {
		return err
	}
	fn(factory)
	factory.Start(c.stopCh)
	return nil
}

func (c *clusterController) getFilteredFactory() (informers.SharedInformerFactory, error) {
	if c.filteredInformerFactory != nil {
		return c.filteredInformerFactory, nil
	}
	var err error
	c.filteredInformerFactory, err = filteredInformerFactory(c.cluster)
	return c.filteredInformerFactory, err
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

func (c *clusterController) createElector(hostID string) (*leaderelection.LeaderElector, error) {
	broadcaster := record.NewBroadcaster()
	recorder := broadcaster.NewRecorder(scheme.Scheme, apiv1.EventSource{
		Component: leaderElectionName,
	})
	lock, err := resourcelock.New(
		resourcelock.EndpointsResourceLock,
		c.cluster.Namespace(),
		leaderElectionName,
		c.cluster.CoreV1(),
		c.cluster.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity:      hostID,
			EventRecorder: recorder,
		},
	)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return le, nil
}

func (c *clusterController) initLeaderElection(ctx context.Context) error {
	id, err := os.Hostname()
	if err != nil {
		return err
	}
	err = ensureNamespace(ctx, c.cluster, c.cluster.Namespace())
	if err != nil {
		return err
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			le, err := c.createElector(id)
			if err != nil {
				log.Errorf("unable to create leader elector: %v", err)
				continue
			}
			le.Run(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}()
	return nil
}

func tsuruOnlyTweakFunc() internalinterfaces.TweakListOptionsFunc {
	return func(opts *metav1.ListOptions) {
		ls := provision.IsServiceLabelSet(tsuruLabelPrefix)
		opts.LabelSelector = labels.SelectorFromSet(labels.Set(ls.ToIsServiceSelector())).String()
	}
}

func filteredInformerFactory(client *ClusterClient) (informers.SharedInformerFactory, error) {
	return InformerFactory(client, tsuruOnlyTweakFunc())
}

type informerFactoryArgs struct {
	restConfig *rest.Config
	resync     time.Duration
	tweak      internalinterfaces.TweakListOptionsFunc
}

func newInformerFactoryArgs(client *ClusterClient, tweak internalinterfaces.TweakListOptionsFunc) *informerFactoryArgs {
	timeout := client.restConfig.Timeout
	restConfig := *client.restConfig
	restConfig.Timeout = 0
	tweakFunc := internalinterfaces.TweakListOptionsFunc(func(opts *metav1.ListOptions) {
		if opts.TimeoutSeconds == nil {
			timeoutSec := int64(timeout.Seconds())
			opts.TimeoutSeconds = &timeoutSec
		}
		if tweak != nil {
			tweak(opts)
		}
	})
	return &informerFactoryArgs{
		restConfig: &restConfig,
		resync:     time.Minute,
		tweak:      tweakFunc,
	}
}

var InformerFactory = func(client *ClusterClient, tweak internalinterfaces.TweakListOptionsFunc) (informers.SharedInformerFactory, error) {
	args := newInformerFactoryArgs(client, tweak)
	cli, err := ClientForConfig(args.restConfig)
	if err != nil {
		return nil, err
	}
	return informers.NewFilteredSharedInformerFactory(cli, args.resync, metav1.NamespaceAll, args.tweak), nil
}
