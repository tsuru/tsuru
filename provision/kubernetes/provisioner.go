// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	"github.com/tsuru/tsuru/provision/node"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/set"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/volume"
	apiv1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	v1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/informers"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/informers/internalinterfaces"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	provisionerName                            = "kubernetes"
	defaultKubeAPITimeout                      = time.Minute
	defaultShortKubeAPITimeout                 = 5 * time.Second
	defaultPodReadyTimeout                     = time.Minute
	defaultPodRunningTimeout                   = 10 * time.Minute
	defaultDeploymentProgressTimeout           = 10 * time.Minute
	defaultAttachTimeoutAfterContainerFinished = time.Minute
	defaultSidecarImageName                    = "tsuru/deploy-agent:0.6.0"
)

type kubernetesProvisioner struct {
	mu               sync.Mutex
	informerFactory  map[string]informers.SharedInformerFactory
	podInformers     map[string]v1informers.PodInformer
	serviceInformers map[string]v1informers.ServiceInformer
	nodeInformers    map[string]v1informers.NodeInformer
	stopCh           chan struct{}
}

var (
	_ provision.Provisioner              = &kubernetesProvisioner{}
	_ provision.NodeProvisioner          = &kubernetesProvisioner{}
	_ provision.NodeContainerProvisioner = &kubernetesProvisioner{}
	_ provision.MessageProvisioner       = &kubernetesProvisioner{}
	_ provision.SleepableProvisioner     = &kubernetesProvisioner{}
	_ provision.VolumeProvisioner        = &kubernetesProvisioner{}
	_ provision.BuilderDeploy            = &kubernetesProvisioner{}
	_ provision.BuilderDeployKubeClient  = &kubernetesProvisioner{}
	_ provision.InitializableProvisioner = &kubernetesProvisioner{}
	_ provision.RollbackableDeployer     = &kubernetesProvisioner{}
	// _ provision.OptionalLogsProvisioner  = &kubernetesProvisioner{}
	// _ provision.UnitStatusProvisioner    = &kubernetesProvisioner{}
	// _ provision.NodeRebalanceProvisioner = &kubernetesProvisioner{}
	// _ provision.AppFilterProvisioner     = &kubernetesProvisioner{}
	// _ builder.PlatformBuilder            = &kubernetesProvisioner{}
	_                         provision.UpdatableProvisioner = &kubernetesProvisioner{}
	mainKubernetesProvisioner *kubernetesProvisioner
)

func init() {
	mainKubernetesProvisioner = &kubernetesProvisioner{
		informerFactory:  make(map[string]informers.SharedInformerFactory),
		podInformers:     make(map[string]v1informers.PodInformer),
		serviceInformers: make(map[string]v1informers.ServiceInformer),
		nodeInformers:    make(map[string]v1informers.NodeInformer),
		stopCh:           make(chan struct{}),
	}
	provision.Register(provisionerName, func() (provision.Provisioner, error) {
		return mainKubernetesProvisioner, nil
	})
	shutdown.Register(mainKubernetesProvisioner)
}

func GetProvisioner() *kubernetesProvisioner {
	return mainKubernetesProvisioner
}

type kubernetesConfig struct {
	LogLevel           int
	DeploySidecarImage string
	DeployInspectImage string
	APITimeout         time.Duration
	APIShortTimeout    time.Duration
	// PodReadyTimeout is the timeout for a pod to become ready after already
	// running.
	PodReadyTimeout time.Duration
	// PodRunningTimeout is the timeout for a pod to become running, should
	// include time necessary to pull remote image.
	PodRunningTimeout time.Duration
	// DeploymentProgressTimeout is the timeout for a deployment to
	// successfully complete.
	DeploymentProgressTimeout time.Duration
	// AttachTimeoutAfterContainerFinished is the time tsuru will wait for an
	// attach call to finish after the attached container has finished.
	AttachTimeoutAfterContainerFinished time.Duration
	// HeadlessServicePort is the port used in headless service, by default the
	// same port number used for container is used.
	HeadlessServicePort int
}

func getKubeConfig() kubernetesConfig {
	conf := kubernetesConfig{}
	conf.LogLevel, _ = config.GetInt("kubernetes:log-level")
	conf.DeploySidecarImage, _ = config.GetString("kubernetes:deploy-sidecar-image")
	if conf.DeploySidecarImage == "" {
		conf.DeploySidecarImage = defaultSidecarImageName
	}
	conf.DeployInspectImage, _ = config.GetString("kubernetes:deploy-inspect-image")
	if conf.DeployInspectImage == "" {
		conf.DeployInspectImage = defaultSidecarImageName
	}
	apiTimeout, _ := config.GetFloat("kubernetes:api-timeout")
	if apiTimeout != 0 {
		conf.APITimeout = time.Duration(apiTimeout * float64(time.Second))
	} else {
		conf.APITimeout = defaultKubeAPITimeout
	}
	apiShortTimeout, _ := config.GetFloat("kubernetes:api-short-timeout")
	if apiShortTimeout != 0 {
		conf.APIShortTimeout = time.Duration(apiShortTimeout * float64(time.Second))
	} else {
		conf.APIShortTimeout = defaultShortKubeAPITimeout
	}
	podReadyTimeout, _ := config.GetFloat("kubernetes:pod-ready-timeout")
	if podReadyTimeout != 0 {
		conf.PodReadyTimeout = time.Duration(podReadyTimeout * float64(time.Second))
	} else {
		conf.PodReadyTimeout = defaultPodReadyTimeout
	}
	podRunningTimeout, _ := config.GetFloat("kubernetes:pod-running-timeout")
	if podRunningTimeout != 0 {
		conf.PodRunningTimeout = time.Duration(podRunningTimeout * float64(time.Second))
	} else {
		conf.PodRunningTimeout = defaultPodRunningTimeout
	}
	deploymentTimeout, _ := config.GetFloat("kubernetes:deployment-progress-timeout")
	if deploymentTimeout != 0 {
		conf.DeploymentProgressTimeout = time.Duration(deploymentTimeout * float64(time.Second))
	} else {
		conf.DeploymentProgressTimeout = defaultDeploymentProgressTimeout
	}
	attachTimeout, _ := config.GetFloat("kubernetes:attach-after-finish-timeout")
	if attachTimeout != 0 {
		conf.AttachTimeoutAfterContainerFinished = time.Duration(attachTimeout * float64(time.Second))
	} else {
		conf.AttachTimeoutAfterContainerFinished = defaultAttachTimeoutAfterContainerFinished
	}
	conf.HeadlessServicePort, _ = config.GetInt("kubernetes:headless-service-port")
	if conf.HeadlessServicePort == 0 {
		conf.HeadlessServicePort, _ = strconv.Atoi(provision.WebProcessDefaultPort())
	}
	return conf
}

func (p *kubernetesProvisioner) Initialize() error {
	conf := getKubeConfig()
	if conf.LogLevel > 0 {
		// These flags are used by golang/glog package which in turn is used by
		// kubernetes to control logging. Unfortunately it doesn't seem like
		// there's a better way to control glog.
		flag.CommandLine.Parse([]string{"-v", strconv.Itoa(conf.LogLevel), "-logtostderr"})
	}
	return nil
}

func (p *kubernetesProvisioner) GetName() string {
	return provisionerName
}

func (p *kubernetesProvisioner) Provision(a provision.App) error {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return err
	}
	return ensureAppCustomResourceSynced(client, a)
}

func (p *kubernetesProvisioner) Destroy(a provision.App) error {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return err
	}
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	app, err := tclient.TsuruV1().Apps(client.Namespace()).Get(a.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := p.removeResources(client, app); err != nil {
		return err
	}
	return tclient.TsuruV1().Apps(client.Namespace()).Delete(a.GetName(), &metav1.DeleteOptions{})
}

func (p *kubernetesProvisioner) removeResources(client *ClusterClient, app *tsuruv1.App) error {
	multiErrors := tsuruErrors.NewMultiError()
	for _, d := range app.Spec.Deployments {
		for _, dd := range d {
			err := client.AppsV1beta2().Deployments(app.Spec.NamespaceName).Delete(dd, &metav1.DeleteOptions{
				PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
			})
			if err != nil && !k8sErrors.IsNotFound(err) {
				multiErrors.Add(err)
			}
		}
	}
	for _, s := range app.Spec.Services {
		for _, ss := range s {
			err := client.CoreV1().Services(app.Spec.NamespaceName).Delete(ss, &metav1.DeleteOptions{
				PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
			})
			if err != nil && !k8sErrors.IsNotFound(err) {
				multiErrors.Add(errors.WithStack(err))
			}
		}
	}
	vols, err := volume.ListByApp(app.Name)
	if err != nil {
		multiErrors.Add(errors.WithStack(err))
	} else {
		for _, vol := range vols {
			_, err = vol.LoadBinds()
			if err != nil {
				continue
			}

			bindedToOtherApps := false
			for _, b := range vol.Binds {
				if b.ID.App != app.Name {
					bindedToOtherApps = true
					break
				}
			}
			if !bindedToOtherApps {
				err = deleteVolume(client, vol.Name)
				if err != nil {
					multiErrors.Add(errors.WithStack(err))
				}
			}
		}
	}
	err = client.CoreV1().ServiceAccounts(app.Spec.NamespaceName).Delete(app.Spec.ServiceAccountName, &metav1.DeleteOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(errors.WithStack(err))
	}
	return multiErrors.ToError()
}

func changeState(a provision.App, process string, state servicecommon.ProcessState, w io.Writer) error {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return err
	}
	if err := ensureAppCustomResourceSynced(client, a); err != nil {
		return err
	}
	return servicecommon.ChangeAppState(&serviceManager{
		client: client,
		writer: w,
	}, a, process, state)
}

func changeUnits(a provision.App, units int, processName string, w io.Writer) error {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return err
	}
	if err := ensureAppCustomResourceSynced(client, a); err != nil {
		return err
	}
	return servicecommon.ChangeUnits(&serviceManager{
		client: client,
		writer: w,
	}, a, units, processName)
}

func (p *kubernetesProvisioner) AddUnits(a provision.App, units uint, processName string, w io.Writer) error {
	return changeUnits(a, int(units), processName, w)
}

func (p *kubernetesProvisioner) RemoveUnits(a provision.App, units uint, processName string, w io.Writer) error {
	return changeUnits(a, -int(units), processName, w)
}

func (p *kubernetesProvisioner) Restart(a provision.App, process string, w io.Writer) error {
	return changeState(a, process, servicecommon.ProcessState{Start: true, Restart: true}, w)
}

func (p *kubernetesProvisioner) Start(a provision.App, process string) error {
	return changeState(a, process, servicecommon.ProcessState{Start: true}, nil)
}

func (p *kubernetesProvisioner) Stop(a provision.App, process string) error {
	return changeState(a, process, servicecommon.ProcessState{Stop: true}, nil)
}

var stateMap = map[apiv1.PodPhase]provision.Status{
	apiv1.PodPending:   provision.StatusCreated,
	apiv1.PodRunning:   provision.StatusStarted,
	apiv1.PodSucceeded: provision.StatusStopped,
	apiv1.PodFailed:    provision.StatusError,
	apiv1.PodUnknown:   provision.StatusError,
}

func (p *kubernetesProvisioner) podsToUnits(client *ClusterClient, pods []apiv1.Pod, baseApp provision.App, baseNode *apiv1.Node) ([]provision.Unit, error) {
	var apps []provision.App
	if baseApp != nil {
		apps = append(apps, baseApp)
	}
	var nodes []apiv1.Node
	if baseNode != nil {
		nodes = append(nodes, *baseNode)
	}
	return p.podsToUnitsMultiple(client, pods, apps, nodes)
}

func (p *kubernetesProvisioner) podsToUnitsMultiple(client *ClusterClient, pods []apiv1.Pod, baseApps []provision.App, baseNodes []apiv1.Node) ([]provision.Unit, error) {
	var err error
	if len(pods) == 0 {
		return nil, nil
	}
	nodeMap := map[string]*apiv1.Node{}
	appMap := map[string]provision.App{}
	portMap := map[string]int32{}
	for _, baseApp := range baseApps {
		appMap[baseApp.GetName()] = baseApp
	}
	if len(baseNodes) == 0 {
		var nodeInformer v1informers.NodeInformer
		nodeInformer, err = p.nodeInformerForCluster(client)
		if err != nil {
			return nil, err
		}
		baseNodes, err = nodesForPods(nodeInformer, pods)
		if err != nil {
			return nil, err
		}
	}
	for i, baseNode := range baseNodes {
		nodeMap[baseNode.Name] = &baseNodes[i]
	}
	svcInformer, err := p.serviceInformerForCluster(client)
	if err != nil {
		return nil, err
	}
	var units []provision.Unit
	for _, pod := range pods {
		if isTerminating(pod) || isEvicted(pod) {
			continue
		}
		l := labelSetFromMeta(&pod.ObjectMeta)
		node, ok := nodeMap[pod.Spec.NodeName]
		if !ok && pod.Spec.NodeName != "" {
			node, err = client.CoreV1().Nodes().Get(pod.Spec.NodeName, metav1.GetOptions{})
			if err != nil {
				return nil, errors.WithStack(err)
			}
			nodeMap[pod.Spec.NodeName] = node
		}
		podApp, ok := appMap[l.AppName()]
		if !ok {
			podApp, err = app.GetByName(l.AppName())
			if err != nil {
				return nil, errors.WithStack(err)
			}
			appMap[podApp.GetName()] = podApp
		}
		wrapper := kubernetesNodeWrapper{node: node, prov: p}
		url := &url.URL{
			Scheme: "http",
			Host:   wrapper.Address(),
		}
		appProcess := l.AppProcess()
		if appProcess != "" {
			srvName := deploymentNameForApp(podApp, appProcess)
			port, ok := portMap[srvName]
			if !ok {
				port, err = getServicePort(svcInformer, srvName, pod.ObjectMeta.Namespace)
				if err != nil && !k8sErrors.IsNotFound(errors.Cause(err)) {
					return nil, err
				}
				portMap[srvName] = port
			}
			if port != 0 {
				url.Host = fmt.Sprintf("%s:%d", url.Host, port)
			}
		}
		units = append(units, provision.Unit{
			ID:          pod.Name,
			Name:        pod.Name,
			AppName:     l.AppName(),
			ProcessName: appProcess,
			Type:        l.AppPlatform(),
			IP:          wrapper.ip(),
			Status:      stateMap[pod.Status.Phase],
			Address:     url,
		})
	}
	return units, nil
}

// merged from https://github.com/kubernetes/kubernetes/blob/1f69c34478800e150acd022f6313a15e1cb7a97c/pkg/quota/evaluator/core/pods.go#L333
// and https://github.com/kubernetes/kubernetes/blob/560e15fb9acee4b8391afbc21fc3aea7b771e2c4/pkg/printers/internalversion/printers.go#L606
func isTerminating(pod apiv1.Pod) bool {
	return pod.Spec.ActiveDeadlineSeconds != nil && *pod.Spec.ActiveDeadlineSeconds >= int64(0) || pod.DeletionTimestamp != nil
}

func isEvicted(pod apiv1.Pod) bool {
	return pod.Status.Phase == apiv1.PodFailed && strings.ToLower(pod.Status.Reason) == "evicted"
}

func nodesForPods(informer v1informers.NodeInformer, pods []apiv1.Pod) ([]apiv1.Node, error) {
	nodeSet := map[string]struct{}{}
	for _, p := range pods {
		nodeSet[p.Spec.NodeName] = struct{}{}
	}
	nodes, err := informer.Lister().List(labels.Everything())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	nodesRet := make([]apiv1.Node, 0, len(nodes))
	for _, node := range nodes {
		_, inSet := nodeSet[node.Name]
		if inSet {
			copy := node.DeepCopy()
			nodesRet = append(nodesRet, *copy)
		}
	}
	return nodesRet, nil
}

func (p *kubernetesProvisioner) Units(apps ...provision.App) ([]provision.Unit, error) {
	cApps, err := clustersForApps(apps)
	if err != nil {
		return nil, err
	}
	var units []provision.Unit
	for _, cApp := range cApps {
		pods, err := p.podsForApps(cApp.client, cApp.apps)
		if err != nil {
			return nil, err
		}
		clusterUnits, err := p.podsToUnitsMultiple(cApp.client, pods, cApp.apps, nil)
		if err != nil {
			return nil, err
		}
		units = append(units, clusterUnits...)
	}
	return units, nil
}

func (p *kubernetesProvisioner) podsForApps(client *ClusterClient, apps []provision.App) ([]apiv1.Pod, error) {
	inSelectorMap := map[string][]string{}
	for _, a := range apps {
		l, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
			App: a,
			ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
				Prefix:      tsuruLabelPrefix,
				Provisioner: provisionerName,
			},
		})
		if err != nil {
			return nil, err
		}
		appSel := l.ToAppSelector()
		for k, v := range appSel {
			inSelectorMap[k] = append(inSelectorMap[k], v)
		}
	}
	sel := labels.NewSelector()
	for k, v := range inSelectorMap {
		if len(v) == 0 {
			continue
		}
		req, err := labels.NewRequirement(k, selection.In, v)
		if err != nil {
			return nil, err
		}
		sel = sel.Add(*req)
	}
	informer, err := p.podInformerForCluster(client)
	if err != nil {
		return nil, err
	}
	pods, err := informer.Lister().List(sel)
	if err != nil {
		return nil, err
	}
	podCopies := make([]apiv1.Pod, len(pods))
	for i, p := range pods {
		podCopies[i] = *p.DeepCopy()
	}
	return podCopies, nil
}

func (p *kubernetesProvisioner) RoutableAddresses(a provision.App) ([]url.URL, error) {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return nil, err
	}
	imageName, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		if err != image.ErrNoImagesAvailable {
			return nil, err
		}
		return nil, nil
	}
	webProcessName, err := image.GetImageWebProcessName(imageName)
	if err != nil {
		return nil, err
	}
	if webProcessName == "" {
		return nil, nil
	}
	srvName := deploymentNameForApp(a, webProcessName)
	ns, err := client.AppNamespace(a)
	if err != nil {
		return nil, err
	}
	svcInformer, err := p.serviceInformerForCluster(client)
	if err != nil {
		return nil, err
	}
	pubPort, err := getServicePort(svcInformer, srvName, ns)
	if err != nil {
		return nil, err
	}
	policyLocal, err := client.ExternalPolicyLocal(a.GetPool())
	if err != nil {
		return nil, err
	}
	if !policyLocal {
		return p.addressesForPool(client, a.GetPool(), pubPort)
	}
	return p.addressesForApp(client, a, pubPort)
}

func (p *kubernetesProvisioner) addressesForApp(client *ClusterClient, a provision.App, pubPort int32) ([]url.URL, error) {
	pods, err := p.podsForApps(client, []provision.App{a})
	if err != nil {
		return nil, err
	}
	addrs := make([]url.URL, 0)
	for _, p := range pods {
		allReady := true
		for _, contStatus := range p.Status.ContainerStatuses {
			if !contStatus.Ready {
				allReady = false
				break
			}
		}
		if allReady {
			addrs = append(addrs, url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s:%d", p.Status.HostIP, pubPort),
			})
		}
	}
	return addrs, nil
}

func (p *kubernetesProvisioner) addressesForPool(client *ClusterClient, poolName string, pubPort int32) ([]url.URL, error) {
	nodeSelector := provision.NodeLabels(provision.NodeLabelsOpts{
		Pool:   poolName,
		Prefix: tsuruLabelPrefix,
	}).ToNodeByPoolSelector()
	nodes, err := client.CoreV1().Nodes().List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(nodeSelector).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	addrs := make([]url.URL, len(nodes.Items))
	for i, n := range nodes.Items {
		wrapper := kubernetesNodeWrapper{node: &n, prov: p}
		addrs[i] = url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s:%d", wrapper.Address(), pubPort),
		}
	}
	return addrs, nil
}

func (p *kubernetesProvisioner) RegisterUnit(a provision.App, unitID string, customData map[string]interface{}) error {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return err
	}
	ns, err := client.AppNamespace(a)
	if err != nil {
		return err
	}
	pod, err := client.CoreV1().Pods(ns).Get(unitID, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return &provision.UnitNotFoundError{ID: unitID}
		}
		return errors.WithStack(err)
	}
	units, err := p.podsToUnits(client, []apiv1.Pod{*pod}, a, nil)
	if err != nil {
		return err
	}
	if len(units) == 0 {
		return errors.Errorf("unable to convert pod to unit: %#v", pod)
	}
	if customData == nil {
		return nil
	}
	l := labelSetFromMeta(&pod.ObjectMeta)
	buildingImage := l.BuildImage()
	if buildingImage == "" {
		return nil
	}
	return errors.WithStack(image.SaveImageCustomData(buildingImage, customData))
}

func (p *kubernetesProvisioner) ListNodes(addressFilter []string) ([]provision.Node, error) {
	var nodes []provision.Node
	kubeConf := getKubeConfig()
	err := forEachCluster(func(c *ClusterClient) error {
		err := c.SetTimeout(kubeConf.APIShortTimeout)
		if err != nil {
			return err
		}
		clusterNodes, err := p.listNodesForCluster(c, addressFilter)
		if err != nil {
			return err
		}
		nodes = append(nodes, clusterNodes...)
		return nil
	})
	if err == provTypes.ErrNoCluster {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (p *kubernetesProvisioner) listNodesForCluster(cluster *ClusterClient, addressFilter []string) ([]provision.Node, error) {
	var nodes []provision.Node
	var addressSet set.Set
	if len(addressFilter) > 0 {
		addressSet = set.FromSlice(addressFilter)
	}
	nodeList, err := cluster.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for i := range nodeList.Items {
		n := &kubernetesNodeWrapper{
			node:    &nodeList.Items[i],
			prov:    p,
			cluster: cluster,
		}
		if addressSet == nil || addressSet.Includes(n.Address()) {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

func (p *kubernetesProvisioner) GetNode(address string) (provision.Node, error) {
	_, node, err := p.findNodeByAddress(address)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func setNodeMetadata(node *apiv1.Node, pool, iaasID string, meta map[string]string) {
	if node.Labels == nil {
		node.Labels = map[string]string{}
	}
	if node.Annotations == nil {
		node.Annotations = map[string]string{}
	}
	for k, v := range meta {
		k = tsuruLabelPrefix + strings.TrimPrefix(k, tsuruLabelPrefix)
		switch k {
		case tsuruExtraAnnotationsMeta:
			appendKV(v, ",", "=", node.Annotations)
		case tsuruExtraLabelsMeta:
			appendKV(v, ",", "=", node.Labels)
		}
		if v == "" {
			delete(node.Annotations, k)
			continue
		}
		node.Annotations[k] = v
	}
	baseNodeLabels := provision.NodeLabels(provision.NodeLabelsOpts{
		IaaSID: iaasID,
		Pool:   pool,
		Prefix: tsuruLabelPrefix,
	})
	for k, v := range baseNodeLabels.ToLabels() {
		if v == "" {
			continue
		}
		delete(node.Annotations, k)
		node.Labels[k] = v
	}
}

func appendKV(s, outSep, innSep string, m map[string]string) {
	kvs := strings.Split(s, outSep)
	for _, kv := range kvs {
		parts := strings.SplitN(kv, innSep, 2)
		if len(parts) != 2 {
			continue
		}
		if parts[1] == "" {
			delete(m, parts[1])
			continue
		}
		m[parts[0]] = parts[1]
	}
}

func (p *kubernetesProvisioner) AddNode(opts provision.AddNodeOptions) error {
	client, err := clusterForPool(opts.Pool)
	if err != nil {
		return err
	}
	hostAddr := tsuruNet.URLToHost(opts.Address)
	node := &apiv1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: hostAddr,
		},
	}
	setNodeMetadata(node, opts.Pool, opts.IaaSID, opts.Metadata)
	_, err = client.CoreV1().Nodes().Create(node)
	if k8sErrors.IsAlreadyExists(err) {
		return p.internalNodeUpdate(provision.UpdateNodeOptions{
			Address:  hostAddr,
			Metadata: opts.Metadata,
			Pool:     opts.Pool,
		}, opts.IaaSID)
	}
	if err == nil {
		servicecommon.RebuildRoutesPoolApps(opts.Pool)
	}
	return err
}

func (p *kubernetesProvisioner) RemoveNode(opts provision.RemoveNodeOptions) error {
	client, nodeWrapper, err := p.findNodeByAddress(opts.Address)
	if err != nil {
		return err
	}
	node := nodeWrapper.node
	if opts.Rebalance {
		node.Spec.Unschedulable = true
		_, err = client.CoreV1().Nodes().Update(node)
		if err != nil {
			return errors.WithStack(err)
		}
		var pods []apiv1.Pod
		pods, err = podsFromNode(client, node.Name, tsuruLabelPrefix+provision.LabelAppPool)
		if err != nil {
			return err
		}
		for _, pod := range pods {
			err = client.CoreV1().Pods(pod.Namespace).Evict(&policy.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pod.Name,
					Namespace: pod.Namespace,
				},
			})
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}
	err = client.CoreV1().Nodes().Delete(node.Name, &metav1.DeleteOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	servicecommon.RebuildRoutesPoolApps(nodeWrapper.Pool())
	return nil
}

func (p *kubernetesProvisioner) NodeForNodeData(nodeData provision.NodeStatusData) (provision.Node, error) {
	return node.FindNodeByAddrs(p, nodeData.Addrs)
}

func (p *kubernetesProvisioner) findNodeByAddress(address string) (*ClusterClient, *kubernetesNodeWrapper, error) {
	var (
		foundNode    *kubernetesNodeWrapper
		foundCluster *ClusterClient
	)
	err := forEachCluster(func(c *ClusterClient) error {
		if foundNode != nil {
			return nil
		}
		node, err := getNodeByAddr(c, address)
		if err == nil {
			foundNode = &kubernetesNodeWrapper{
				node:    node,
				prov:    p,
				cluster: c,
			}
			foundCluster = c
			return nil
		}
		if err != provision.ErrNodeNotFound {
			return err
		}
		return nil
	})
	if err != nil {
		if err == provTypes.ErrNoCluster {
			return nil, nil, provision.ErrNodeNotFound
		}
		return nil, nil, err
	}
	if foundNode == nil {
		return nil, nil, provision.ErrNodeNotFound
	}
	return foundCluster, foundNode, nil
}

func (p *kubernetesProvisioner) UpdateNode(opts provision.UpdateNodeOptions) error {
	return p.internalNodeUpdate(opts, "")
}

func (p *kubernetesProvisioner) internalNodeUpdate(opts provision.UpdateNodeOptions, iaasID string) error {
	client, nodeWrapper, err := p.findNodeByAddress(opts.Address)
	if err != nil {
		return err
	}
	if nodeWrapper.IaaSID() != "" {
		iaasID = ""
	}
	node := nodeWrapper.node
	shouldRemove := map[string]bool{
		tsuruInProgressTaint:   true,
		tsuruNodeDisabledTaint: opts.Enable,
	}
	taints := node.Spec.Taints
	var isDisabled bool
	for i := 0; i < len(taints); i++ {
		if taints[i].Key == tsuruNodeDisabledTaint {
			isDisabled = true
		}
		if remove := shouldRemove[taints[i].Key]; remove {
			taints[i] = taints[len(taints)-1]
			taints = taints[:len(taints)-1]
			i--
		}
	}
	if !isDisabled && opts.Disable {
		taints = append(taints, apiv1.Taint{
			Key:    tsuruNodeDisabledTaint,
			Effect: apiv1.TaintEffectNoSchedule,
		})
	}
	node.Spec.Taints = taints
	setNodeMetadata(node, opts.Pool, iaasID, opts.Metadata)
	_, err = client.CoreV1().Nodes().Update(node)
	return errors.WithStack(err)
}

func (p *kubernetesProvisioner) Deploy(a provision.App, buildImageID string, evt *event.Event) (string, error) {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return "", err
	}
	if err = ensureAppCustomResourceSynced(client, a); err != nil {
		return "", err
	}
	newImage := buildImageID
	if strings.HasSuffix(buildImageID, "-builder") {
		newImage, err = image.AppNewImageName(a.GetName())
		if err != nil {
			return "", err
		}
		var deployPodName string
		deployPodName, err = deployPodNameForApp(a)
		if err != nil {
			return "", err
		}
		ns, nsErr := client.AppNamespace(a)
		if nsErr != nil {
			return "", nsErr
		}
		defer cleanupPod(client, deployPodName, ns)
		params := createPodParams{
			app:               a,
			client:            client,
			podName:           deployPodName,
			sourceImage:       buildImageID,
			destinationImages: []string{newImage},
			attachOutput:      evt,
			attachInput:       strings.NewReader("."),
			inputFile:         "/dev/null",
		}
		ctx, cancel := evt.CancelableContext(context.Background())
		err = createDeployPod(ctx, params)
		cancel()
		if err != nil {
			return "", err
		}
	}
	manager := &serviceManager{
		client: client,
		writer: evt,
	}
	err = servicecommon.RunServicePipeline(manager, a, newImage, nil, evt)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return newImage, ensureAppCustomResourceSynced(client, a)
}

func (p *kubernetesProvisioner) Rollback(a provision.App, imageID string, evt *event.Event) (string, error) {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return "", err
	}
	foundImageID, err := image.GetAppImageBySuffix(a.GetName(), imageID)
	if err != nil {
		return "", err
	}
	imgMetaData, err := image.GetImageMetaData(foundImageID)
	if err != nil {
		return "", err
	}
	if imgMetaData.DisableRollback {
		return "", fmt.Errorf("Can't Rollback image %s, reason: %s", foundImageID, imgMetaData.Reason)
	}
	manager := &serviceManager{
		client: client,
		writer: evt,
	}
	err = servicecommon.RunServicePipeline(manager, a, foundImageID, nil, evt)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return imageID, nil
}

func (p *kubernetesProvisioner) UpgradeNodeContainer(name string, pool string, writer io.Writer) error {
	m := nodeContainerManager{}
	return servicecommon.UpgradeNodeContainer(&m, name, pool, writer)
}

func (p *kubernetesProvisioner) RemoveNodeContainer(name string, pool string, writer io.Writer) error {
	err := forEachCluster(func(cluster *ClusterClient) error {
		return cleanupDaemonSet(cluster, name, pool)
	})
	if err == provTypes.ErrNoCluster {
		return nil
	}
	return err
}

func (p *kubernetesProvisioner) ExecuteCommand(opts provision.ExecOptions) error {
	client, err := clusterForPool(opts.App.GetPool())
	if err != nil {
		return err
	}
	var size *remotecommand.TerminalSize
	if opts.Width != 0 && opts.Height != 0 {
		size = &remotecommand.TerminalSize{
			Width:  uint16(opts.Width),
			Height: uint16(opts.Height),
		}
	}
	if opts.Term != "" {
		opts.Cmds = append([]string{"/usr/bin/env", "TERM=" + opts.Term}, opts.Cmds...)
	}
	eOpts := execOpts{
		client:   client,
		app:      opts.App,
		cmds:     opts.Cmds,
		stdout:   opts.Stdout,
		stderr:   opts.Stderr,
		stdin:    opts.Stdin,
		termSize: size,
		tty:      opts.Stdin != nil,
	}
	if len(opts.Units) == 0 {
		return runIsolatedCmdPod(client, eOpts)
	}
	for _, u := range opts.Units {
		eOpts.unit = u
		err := execCommand(eOpts)
		if err != nil {
			return err
		}
	}
	return nil
}

func runIsolatedCmdPod(client *ClusterClient, opts execOpts) error {
	baseName := execCommandPodNameForApp(opts.app)
	labels, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App: opts.app,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:        tsuruLabelPrefix,
			Provisioner:   provisionerName,
			IsIsolatedRun: true,
		},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	if opts.image == "" {
		opts.image, err = image.AppCurrentImageName(opts.app.GetName())
		if err != nil {
			return err
		}
	}
	appEnvs := provision.EnvsForApp(opts.app, "", false)
	var envs []apiv1.EnvVar
	for _, envData := range appEnvs {
		envs = append(envs, apiv1.EnvVar{Name: envData.Name, Value: envData.Value})
	}
	return runPod(runSinglePodArgs{
		client:   client,
		stdout:   opts.stdout,
		stderr:   opts.stderr,
		stdin:    opts.stdin,
		termSize: opts.termSize,
		image:    opts.image,
		labels:   labels,
		cmds:     opts.cmds,
		envs:     envs,
		name:     baseName,
		app:      opts.app,
	})
}

func (p *kubernetesProvisioner) StartupMessage() (string, error) {
	clusters, err := allClusters()
	if err != nil {
		if err == provTypes.ErrNoCluster {
			return "", nil
		}
		return "", err
	}
	var out string
	for _, c := range clusters {
		nodeList, err := p.listNodesForCluster(c, nil)
		if err != nil {
			return "", err
		}
		out += fmt.Sprintf("Kubernetes provisioner on cluster %q - %s:\n", c.Name, c.restConfig.Host)
		if len(nodeList) == 0 {
			out += "    No Kubernetes nodes available\n"
		}
		for _, node := range nodeList {
			out += fmt.Sprintf("    Kubernetes node: %s\n", node.Address())
		}
	}
	return out, nil
}

func (p *kubernetesProvisioner) Sleep(a provision.App, process string) error {
	return changeState(a, process, servicecommon.ProcessState{Stop: true, Sleep: true}, nil)
}

func (p *kubernetesProvisioner) DeleteVolume(volumeName, pool string) error {
	client, err := clusterForPool(pool)
	if err != nil {
		return err
	}
	return deleteVolume(client, volumeName)
}

func (p *kubernetesProvisioner) IsVolumeProvisioned(volumeName, pool string) (bool, error) {
	client, err := clusterForPool(pool)
	if err != nil {
		return false, err
	}
	return volumeExists(client, volumeName)
}

func (p *kubernetesProvisioner) UpdateApp(old, new provision.App, w io.Writer) error {
	if old.GetPool() == new.GetPool() {
		return nil
	}
	client, err := clusterForPool(old.GetPool())
	if err != nil {
		return err
	}
	newclient, err := clusterForPool(new.GetPool())
	if err != nil {
		return err
	}
	sameCluster := client.GetCluster().Name == newclient.GetCluster().Name
	sameNamespace := client.PoolNamespace(old.GetPool()) == client.PoolNamespace(new.GetPool())
	if sameCluster && !sameNamespace {
		volumes, err := volume.ListByApp(old.GetName())
		if err != nil {
			return err
		}
		if len(volumes) > 0 {
			return fmt.Errorf("can't change the pool of an app with binded volumes")
		}
	}
	params := updatePipelineParams{
		old: old,
		new: new,
		w:   w,
		p:   p,
	}
	if !sameCluster {
		actions := []*action.Action{
			&provisionNewApp,
			&restartApp,
			&rebuildAppRoutes,
			&destroyOldApp,
		}
		return action.NewPipeline(actions...).Execute(params)
	}
	// same cluster and it is not configured with per-pool-namespace, nothing to do.
	if sameNamespace {
		return nil
	}
	actions := []*action.Action{
		&updateAppCR,
		&restartApp,
		&rebuildAppRoutes,
		&removeOldAppResources,
	}
	return action.NewPipeline(actions...).Execute(params)
}

func (p *kubernetesProvisioner) Shutdown(ctx context.Context) error {
	close(p.stopCh)
	return nil
}

func (p *kubernetesProvisioner) podInformerForCluster(client *ClusterClient) (v1informers.PodInformer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if informer, ok := p.podInformers[client.Name]; ok {
		return informer, nil
	}
	err := p.withInformerFactory(client, func(factory informers.SharedInformerFactory) {
		p.podInformers[client.Name] = factory.Core().V1().Pods()
		p.podInformers[client.Name].Informer()
	})
	return p.podInformers[client.Name], err
}

func (p *kubernetesProvisioner) serviceInformerForCluster(client *ClusterClient) (v1informers.ServiceInformer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if informer, ok := p.serviceInformers[client.Name]; ok {
		return informer, nil
	}
	err := p.withInformerFactory(client, func(factory informers.SharedInformerFactory) {
		p.serviceInformers[client.Name] = factory.Core().V1().Services()
		p.serviceInformers[client.Name].Informer()
	})
	return p.serviceInformers[client.Name], err
}

func (p *kubernetesProvisioner) nodeInformerForCluster(client *ClusterClient) (v1informers.NodeInformer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if informer, ok := p.nodeInformers[client.Name]; ok {
		return informer, nil
	}
	err := p.withInformerFactory(client, func(factory informers.SharedInformerFactory) {
		p.nodeInformers[client.Name] = factory.Core().V1().Nodes()
		p.nodeInformers[client.Name].Informer()
	})
	return p.nodeInformers[client.Name], err
}

func (p *kubernetesProvisioner) withInformerFactory(client *ClusterClient, fn func(factory informers.SharedInformerFactory)) error {
	factory, err := p.factoryForCluster(client)
	if err != nil {
		return err
	}
	fn(factory)
	factory.Start(p.stopCh)
	factory.WaitForCacheSync(p.stopCh)
	return nil
}

func (p *kubernetesProvisioner) factoryForCluster(client *ClusterClient) (informers.SharedInformerFactory, error) {
	if factory, ok := p.informerFactory[client.Name]; ok {
		return factory, nil
	}
	var err error
	p.informerFactory[client.Name], err = InformerFactory(client, p.stopCh)
	return p.informerFactory[client.Name], err
}

var InformerFactory = func(client *ClusterClient, stopCh <-chan struct{}) (informers.SharedInformerFactory, error) {
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

func ensureAppCustomResourceSynced(client *ClusterClient, a provision.App) error {
	err := ensureNamespace(client, client.Namespace())
	if err != nil {
		return err
	}
	err = ensureAppCustomResource(client, a)
	if err != nil {
		return err
	}
	curImg, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return err
	}
	currentImageData, err := image.GetImageMetaData(curImg)
	if err != nil {
		return err
	}
	deployments := make(map[string][]string)
	services := make(map[string][]string)
	for p := range currentImageData.Processes {
		deployments[p] = append(deployments[p], deploymentNameForApp(a, p))
		services[p] = append(services[p], deploymentNameForApp(a, p), headlessServiceNameForApp(a, p))
	}
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	appCRD, err := tclient.TsuruV1().Apps(client.Namespace()).Get(a.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}
	appCRD.Spec.Services = services
	appCRD.Spec.Deployments = deployments
	appCRD.Spec.ServiceAccountName = serviceAccountNameForApp(a)
	_, err = tclient.TsuruV1().Apps(client.Namespace()).Update(appCRD)
	return err
}

func ensureAppCustomResource(client *ClusterClient, a provision.App) error {
	err := ensureCustomResourceDefinitions(client)
	if err != nil {
		return err
	}
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	_, err = tclient.TsuruV1().Apps(client.Namespace()).Get(a.GetName(), metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !k8sErrors.IsNotFound(err) {
		return err
	}
	_, err = tclient.TsuruV1().Apps(client.Namespace()).Create(&tsuruv1.App{
		ObjectMeta: metav1.ObjectMeta{Name: a.GetName()},
		Spec:       tsuruv1.AppSpec{NamespaceName: client.PoolNamespace(a.GetPool())},
	})
	return err
}

func ensureCustomResourceDefinitions(client *ClusterClient) error {
	extClient, err := ExtensionsClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	toCreate := appCustomResourceDefinition()
	_, err = extClient.ApiextensionsV1beta1().CustomResourceDefinitions().Create(toCreate)
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return err
	}
	timeout := time.After(time.Minute)
loop:
	for {
		crd, errGet := extClient.ApiextensionsV1beta1().CustomResourceDefinitions().Get(toCreate.GetName(), metav1.GetOptions{})
		if errGet != nil {
			return errGet
		}
		for _, c := range crd.Status.Conditions {
			if c.Type == v1beta1.Established && c.Status == v1beta1.ConditionTrue {
				break loop
			}
		}
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for custom resource definition creation")
		case <-time.After(time.Second):
		}
	}
	return nil
}

func appCustomResourceDefinition() *v1beta1.CustomResourceDefinition {
	return &v1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "apps.tsuru.io"},
		Spec: v1beta1.CustomResourceDefinitionSpec{
			Group:   "tsuru.io",
			Version: "v1",
			Names: v1beta1.CustomResourceDefinitionNames{
				Plural:   "apps",
				Singular: "app",
				Kind:     "App",
				ListKind: "AppList",
			},
		},
	}
}
