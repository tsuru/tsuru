// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/provision/node"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/set"
	apiv1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	provisionerName                  = "kubernetes"
	defaultKubeAPITimeout            = time.Minute
	defaultShortKubeAPITimeout       = 5 * time.Second
	defaultPodReadyTimeout           = time.Minute
	defaultPodRunningTimeout         = 10 * time.Minute
	defaultDeploymentProgressTimeout = 10 * time.Minute
	defaultSidecarImageName          = "tsuru/deploy-agent:0.4.0"
)

type kubernetesProvisioner struct{}

var (
	_ provision.Provisioner              = &kubernetesProvisioner{}
	_ provision.ShellProvisioner         = &kubernetesProvisioner{}
	_ provision.NodeProvisioner          = &kubernetesProvisioner{}
	_ provision.NodeContainerProvisioner = &kubernetesProvisioner{}
	_ provision.ExecutableProvisioner    = &kubernetesProvisioner{}
	_ provision.MessageProvisioner       = &kubernetesProvisioner{}
	_ provision.SleepableProvisioner     = &kubernetesProvisioner{}
	_ provision.VolumeProvisioner        = &kubernetesProvisioner{}
	_ provision.BuilderDeploy            = &kubernetesProvisioner{}
	_ provision.BuilderDeployKubeClient  = &kubernetesProvisioner{}
	// _ provision.InitializableProvisioner = &kubernetesProvisioner{}
	_ provision.RollbackableDeployer = &kubernetesProvisioner{}
	// _ provision.OptionalLogsProvisioner  = &kubernetesProvisioner{}
	// _ provision.UnitStatusProvisioner    = &kubernetesProvisioner{}
	// _ provision.NodeRebalanceProvisioner = &kubernetesProvisioner{}
	// _ provision.AppFilterProvisioner     = &kubernetesProvisioner{}
	// _ builder.PlatformBuilder            = &kubernetesProvisioner{}
)

func init() {
	provision.Register(provisionerName, func() (provision.Provisioner, error) {
		return &kubernetesProvisioner{}, nil
	})
}

func GetProvisioner() *kubernetesProvisioner {
	return &kubernetesProvisioner{}
}

type kubernetesConfig struct {
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
}

func getKubeConfig() kubernetesConfig {
	conf := kubernetesConfig{}
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
	return conf
}

func (p *kubernetesProvisioner) GetName() string {
	return provisionerName
}

func (p *kubernetesProvisioner) Provision(provision.App) error {
	return nil
}

func (p *kubernetesProvisioner) Destroy(a provision.App) error {
	imgID, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return errors.WithStack(err)
	}
	data, err := image.GetImageMetaData(imgID)
	if err != nil {
		return errors.WithStack(err)
	}
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return err
	}
	manager := &serviceManager{
		client: client,
	}
	multiErrors := tsuruErrors.NewMultiError()
	for process := range data.Processes {
		err = manager.RemoveService(a, process)
		if err != nil {
			multiErrors.Add(err)
		}
	}
	if multiErrors.Len() > 0 {
		return multiErrors
	}
	return nil
}

func changeState(a provision.App, process string, state servicecommon.ProcessState, w io.Writer) error {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
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
	webProcMap := map[string]string{}
	portMap := map[string]int32{}
	for _, baseApp := range baseApps {
		appMap[baseApp.GetName()] = baseApp
	}
	if len(baseNodes) == 0 {
		baseNodes, err = nodesForPods(client, pods)
		if err != nil {
			return nil, err
		}
	}
	for i, baseNode := range baseNodes {
		nodeMap[baseNode.Name] = &baseNodes[i]
	}
	var units []provision.Unit
	for _, pod := range pods {
		if isTerminating(pod) {
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
		webProcessName, ok := webProcMap[podApp.GetName()]
		if !ok {
			var imageName string
			imageName, err = image.AppCurrentImageName(podApp.GetName())
			if err != nil {
				return nil, errors.WithStack(err)
			}
			webProcessName, err = image.GetImageWebProcessName(imageName)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			webProcMap[podApp.GetName()] = webProcessName
		}
		wrapper := kubernetesNodeWrapper{node: node, prov: p}
		url := &url.URL{
			Scheme: "http",
			Host:   wrapper.Address(),
		}
		appProcess := l.AppProcess()
		if appProcess != "" && appProcess == webProcessName {
			srvName := deploymentNameForApp(podApp, webProcessName)
			port, ok := portMap[srvName]
			if !ok {
				port, err = getServicePort(client, srvName)
				if err != nil {
					return nil, err
				}
				portMap[srvName] = port
			}
			url.Host = fmt.Sprintf("%s:%d", url.Host, port)
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

func nodesForPods(client *ClusterClient, pods []apiv1.Pod) ([]apiv1.Node, error) {
	nodeSet := map[string]struct{}{}
	for _, p := range pods {
		nodeSet[p.Spec.NodeName] = struct{}{}
	}
	nodeNames := make([]string, 0, len(nodeSet))
	for nodeName := range nodeSet {
		nodeNames = append(nodeNames, nodeName)
	}
	req, err := labels.NewRequirement("metadata.name", selection.In, nodeNames)
	if err != nil {
		return nil, err
	}
	sel := labels.NewSelector()
	sel.Add(*req)
	nodes, err := client.CoreV1().Nodes().List(metav1.ListOptions{
		FieldSelector: sel.String(),
	})
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

func (p *kubernetesProvisioner) Units(apps ...provision.App) ([]provision.Unit, error) {
	cApps, err := clustersForApps(apps)
	if err != nil {
		return nil, err
	}
	var units []provision.Unit
	for _, cApp := range cApps {
		kubeConf := getKubeConfig()
		err = cApp.client.SetTimeout(kubeConf.APIShortTimeout)
		if err != nil {
			return nil, err
		}
		pods, err := podsForApps(cApp.client, cApp.apps)
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

func podsForApps(client *ClusterClient, apps []provision.App) ([]apiv1.Pod, error) {
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
	pods, err := client.CoreV1().Pods(client.Namespace()).List(metav1.ListOptions{
		LabelSelector: sel.String(),
	})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
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
	pubPort, err := getServicePort(client, srvName)
	if err != nil {
		return nil, err
	}
	nodeSelector := provision.NodeLabels(provision.NodeLabelsOpts{
		Pool:   a.GetPool(),
		Prefix: tsuruLabelPrefix,
	}).ToNodeByPoolSelector()
	nodes, err := client.CoreV1().Nodes().List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(nodeSelector).String(),
	})
	if err != nil {
		return nil, err
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
	pod, err := client.CoreV1().Pods(client.Namespace()).Get(unitID, metav1.GetOptions{})
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
	if err == cluster.ErrNoCluster {
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
		return nil, err
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
		if v == "" {
			delete(node.Annotations, k)
		} else {
			node.Annotations[k] = v
		}
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
	if node.Spec.ProviderID == "" {
		node.Spec.ProviderID = iaasID
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
		pods, err = podsFromNode(client, node.Name, "")
		if err != nil {
			return err
		}
		for _, pod := range pods {
			err = client.CoreV1().Pods(client.Namespace()).Evict(&policy.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pod.Name,
					Namespace: client.Namespace(),
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
		if err == cluster.ErrNoCluster {
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
	return err
}

func (p *kubernetesProvisioner) Deploy(a provision.App, buildImageID string, evt *event.Event) (string, error) {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
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
		defer cleanupPod(client, deployPodName)
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
	return newImage, nil
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
	if err == cluster.ErrNoCluster {
		return nil
	}
	return err
}

func (p *kubernetesProvisioner) Shell(opts provision.ShellOptions) error {
	return execCommand(execOpts{
		app:    opts.App,
		unit:   opts.Unit,
		cmds:   []string{"/usr/bin/env", "TERM=" + opts.Term, "bash", "-l"},
		stdout: opts.Conn,
		stderr: opts.Conn,
		stdin:  opts.Conn,
		termSize: &remotecommand.TerminalSize{
			Width:  uint16(opts.Width),
			Height: uint16(opts.Height),
		},
		tty: true,
	})
}

func (p *kubernetesProvisioner) ExecuteCommand(stdout, stderr io.Writer, a provision.App, cmd string, args ...string) error {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return err
	}
	l, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	pods, err := client.CoreV1().Pods(client.Namespace()).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(l.ToAppSelector())).String(),
	})
	if err != nil {
		return errors.WithStack(err)
	}
	if len(pods.Items) == 0 {
		return provision.ErrEmptyApp
	}
	for _, pod := range pods.Items {
		err = execCommand(execOpts{
			unit:   pod.Name,
			app:    a,
			cmds:   append([]string{"/bin/sh", "-lc", cmd}, args...),
			stdout: stdout,
			stderr: stderr,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *kubernetesProvisioner) ExecuteCommandOnce(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	return execCommand(execOpts{
		app:    app,
		cmds:   append([]string{"/bin/sh", "-lc", cmd}, args...),
		stdout: stdout,
		stderr: stderr,
	})
}

func runIsolatedCmdPod(client *ClusterClient, a provision.App, out, errW io.Writer, cmds []string) error {
	baseName := execCommandPodNameForApp(a)
	labels, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:        tsuruLabelPrefix,
			Provisioner:   provisionerName,
			IsIsolatedRun: true,
		},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	imgName, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return err
	}
	appEnvs := provision.EnvsForApp(a, "", false)
	var envs []apiv1.EnvVar
	for _, envData := range appEnvs {
		envs = append(envs, apiv1.EnvVar{Name: envData.Name, Value: envData.Value})
	}
	return runPod(runSinglePodArgs{
		client: client,
		stdout: out,
		stderr: errW,
		labels: labels,
		cmd:    strings.Join(cmds, " "),
		envs:   envs,
		name:   baseName,
		image:  imgName,
		app:    a,
	})
}

func (p *kubernetesProvisioner) ExecuteCommandIsolated(stdout, stderr io.Writer, a provision.App, cmd string, args ...string) error {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return err
	}
	cmds := append([]string{cmd}, args...)
	return runIsolatedCmdPod(client, a, stdout, stderr, cmds)
}

func (p *kubernetesProvisioner) StartupMessage() (string, error) {
	clusters, err := allClusters()
	if err != nil {
		if err == cluster.ErrNoCluster {
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
