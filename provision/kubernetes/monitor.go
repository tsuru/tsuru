// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	vpaInformers "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions"
	vpaV1Informers "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions/autoscaling.k8s.io/v1"
	vpaInternalInterfaces "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions/internalinterfaces"
	"k8s.io/client-go/informers"
	autoscalingInformers "k8s.io/client-go/informers/autoscaling/v2beta2"
	jobsInformer "k8s.io/client-go/informers/batch/v1"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/informers/internalinterfaces"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	informerSyncTimeout = 10 * time.Second
)

type podListener interface {
	OnPodEvent(pod *apiv1.Pod)
}

type clusterController struct {
	mu                      sync.Mutex
	cluster                 *ClusterClient
	informerFactory         informers.SharedInformerFactory
	filteredInformerFactory informers.SharedInformerFactory
	jobInformerFactory      informers.SharedInformerFactory
	vpaInformerFactory      vpaInformers.SharedInformerFactory
	podInformer             v1informers.PodInformer
	serviceInformer         v1informers.ServiceInformer
	hpaInformer             autoscalingInformers.HorizontalPodAutoscalerInformer
	vpaInformer             vpaV1Informers.VerticalPodAutoscalerInformer
	jobsInformer            jobsInformer.JobInformer
	eventsInformer          v1informers.EventInformer
	stopCh                  chan struct{}
	cancel                  context.CancelFunc
	startedAt               time.Time
	podListeners            map[string]podListener
	podListenersMu          sync.RWMutex
	wg                      sync.WaitGroup
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
		cluster:      cluster,
		stopCh:       make(chan struct{}),
		cancel:       cancel,
		startedAt:    time.Now(),
		podListeners: make(map[string]podListener),
	}
	err := c.initLeaderElection(ctx)
	if err != nil {
		c.stop(ctx)
		return nil, err
	}
	_, err = c.start()
	if err != nil {
		c.stop(ctx)
		return nil, err
	}
	if enableJobEvents, _ := c.cluster.EnableJobEventCreation(); enableJobEvents {
		err = c.startJobInformer()
		if err != nil {
			c.stop(ctx)
			return nil, err
		}
	}
	p.clusterControllers[cluster.Name] = c
	return c, nil
}

func stopClusterController(ctx context.Context, p *kubernetesProvisioner, cluster *ClusterClient) {
	stopClusterControllerByName(ctx, p, cluster.Name)
}

func stopClusterControllerByName(ctx context.Context, p *kubernetesProvisioner, clusterName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clusterControllers[clusterName]; ok {
		c.stop(ctx)
	}
	delete(p.clusterControllers, clusterName)
}

func (c *clusterController) stop(ctx context.Context) {
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

func (c *clusterController) startJobInformer() error {
	eventsInformer, err := c.getEventInformerWait(false)
	if err != nil {
		return err
	}
	eventsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			evt, ok := obj.(*apiv1.Event)
			if !ok {
				return
			}
			if evt.InvolvedObject.Kind != "Job" {
				return
			}
			jobInformer, err := c.getJobInformer()
			if err != nil {
				return
			}
			job, err := jobInformer.Lister().Jobs(evt.Namespace).Get(evt.InvolvedObject.Name)
			if err != nil {
				return
			}
			createJobEvent(job, evt)
		},
	})

	return nil
}

func (c *clusterController) start() (v1informers.PodInformer, error) {
	informer, err := c.getPodInformerWait(false)
	if err != nil {
		return nil, err
	}

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

	return informer, nil
}

func (c *clusterController) notifyPodChanges(pod *apiv1.Pod) {
	c.podListenersMu.RLock()
	defer c.podListenersMu.RUnlock()

	for _, listener := range c.podListeners {
		listener.OnPodEvent(pod)
	}
}

func (c *clusterController) addPodListener(key string, listener podListener) {
	c.podListenersMu.Lock()
	defer c.podListenersMu.Unlock()

	c.podListeners[key] = listener
}

func (c *clusterController) removePodListener(key string) {
	c.podListenersMu.Lock()
	defer c.podListenersMu.Unlock()

	delete(c.podListeners, key)
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
			factory, err := VPAInformerFactory(c.cluster)
			if err != nil {
				return nil, err
			}
			c.vpaInformerFactory = factory
		}
		c.vpaInformer = c.vpaInformerFactory.Autoscaling().V1().VerticalPodAutoscalers()
		c.vpaInformer.Informer()
		c.vpaInformerFactory.Start(c.stopCh)
	}
	err := c.waitForSync(c.vpaInformer.Informer())
	return c.vpaInformer, err
}

func (c *clusterController) getJobInformer() (jobsInformer.JobInformer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.jobsInformer == nil {
		err := c.withJobInformerFactory(func(factory informers.SharedInformerFactory) {
			c.jobsInformer = factory.Batch().V1().Jobs()
			c.jobsInformer.Informer()
		})
		if err != nil {
			return nil, err
		}
	}
	err := c.waitForSync(c.jobsInformer.Informer())
	return c.jobsInformer, err
}

func (c *clusterController) getEventInformerWait(wait bool) (v1informers.EventInformer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.eventsInformer == nil {
		err := c.withInformerFactory(func(factory informers.SharedInformerFactory) {
			c.eventsInformer = factory.Core().V1().Events()
			c.eventsInformer.Informer()
		})
		if err != nil {
			return nil, err
		}
	}
	var err error
	if wait {
		err = c.waitForSync(c.eventsInformer.Informer())
	}
	return c.eventsInformer, err
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

func (c *clusterController) withFilteredInformerFactory(fn func(factory informers.SharedInformerFactory)) error {
	factory, err := c.getFilteredFactory()
	if err != nil {
		return err
	}
	fn(factory)
	factory.Start(c.stopCh)
	return nil
}

func (c *clusterController) withJobInformerFactory(fn func(factory informers.SharedInformerFactory)) error {
	factory, err := c.getJobFactory()
	if err != nil {
		return err
	}
	fn(factory)
	factory.Start(c.stopCh)
	return nil
}

func (c *clusterController) getJobFactory() (informers.SharedInformerFactory, error) {
	if c.jobInformerFactory != nil {
		return c.jobInformerFactory, nil
	}
	var err error
	c.jobInformerFactory, err = InformerFactory(c.cluster, tsuruJobTweakFunc())
	return c.jobInformerFactory, err
}

func (c *clusterController) getFactory() (informers.SharedInformerFactory, error) {
	if c.informerFactory != nil {
		return c.informerFactory, nil
	}
	var err error
	c.informerFactory, err = InformerFactory(c.cluster, nil)
	return c.informerFactory, err
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

func (c *clusterController) initLeaderElection(ctx context.Context) error {
	err := ensureNamespace(ctx, c.cluster, c.cluster.Namespace())
	if err != nil {
		return err
	}

	return nil
}

func tsuruServiceTweakFunc() internalinterfaces.TweakListOptionsFunc {
	return func(opts *metav1.ListOptions) {
		ls := provision.ServiceLabelSet(tsuruLabelPrefix)
		opts.LabelSelector = labels.SelectorFromSet(labels.Set(ls.ToIsServiceSelector())).String()
	}
}

func tsuruJobTweakFunc() internalinterfaces.TweakListOptionsFunc {
	return func(opts *metav1.ListOptions) {
		ls := provision.TsuruJobLabelSet(tsuruLabelPrefix)
		opts.LabelSelector = labels.SelectorFromSet(labels.Set(ls.ToIsServiceSelector())).String()
	}
}

func filteredInformerFactory(client *ClusterClient) (informers.SharedInformerFactory, error) {
	return InformerFactory(client, tsuruServiceTweakFunc())
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

var VPAInformerFactory = func(client *ClusterClient) (vpaInformers.SharedInformerFactory, error) {
	args := newInformerFactoryArgs(client, tsuruServiceTweakFunc())
	cli, err := VPAClientForConfig(args.restConfig)
	if err != nil {
		return nil, err
	}
	tweak := vpaInternalInterfaces.TweakListOptionsFunc(args.tweak)
	return vpaInformers.NewFilteredSharedInformerFactory(cli, args.resync, metav1.NamespaceAll, tweak), nil
}
