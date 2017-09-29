// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
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
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/set"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	policy "k8s.io/client-go/pkg/apis/policy/v1beta1"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	provisionerName                  = "kubernetes"
	defaultKubeAPITimeout            = time.Minute
	defaultShortKubeAPITimeout       = 5 * time.Second
	defaultPodReadyTimeout           = time.Minute
	defaultPodRunningTimeout         = 10 * time.Minute
	defaultDeploymentProgressTimeout = 10 * time.Minute
	defaultDockerImageName           = "docker:1.11.2"
)

type kubernetesProvisioner struct{}

var (
	_ provision.Provisioner              = &kubernetesProvisioner{}
	_ provision.UploadDeployer           = &kubernetesProvisioner{}
	_ provision.ShellProvisioner         = &kubernetesProvisioner{}
	_ provision.NodeProvisioner          = &kubernetesProvisioner{}
	_ provision.NodeContainerProvisioner = &kubernetesProvisioner{}
	_ provision.ExecutableProvisioner    = &kubernetesProvisioner{}
	_ provision.MessageProvisioner       = &kubernetesProvisioner{}
	_ provision.SleepableProvisioner     = &kubernetesProvisioner{}
	_ provision.ImageDeployer            = &kubernetesProvisioner{}
	_ provision.VolumeProvisioner        = &kubernetesProvisioner{}
	// _ provision.ArchiveDeployer          = &kubernetesProvisioner{}
	// _ provision.InitializableProvisioner = &kubernetesProvisioner{}
	// _ provision.RollbackableDeployer     = &kubernetesProvisioner{}
	// _ provision.RebuildableDeployer      = &kubernetesProvisioner{}
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
		conf.DeploySidecarImage = defaultDockerImageName
	}
	conf.DeployInspectImage, _ = config.GetString("kubernetes:deploy-inspect-image")
	if conf.DeployInspectImage == "" {
		conf.DeployInspectImage = defaultDockerImageName
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

func (p *kubernetesProvisioner) podsToUnits(client *clusterClient, pods []apiv1.Pod, baseApp provision.App, baseNode *apiv1.Node) ([]provision.Unit, error) {
	var err error
	if len(pods) == 0 {
		return nil, nil
	}
	nodeMap := map[string]*apiv1.Node{}
	appMap := map[string]provision.App{}
	webProcMap := map[string]string{}
	portMap := map[string]int32{}
	if baseApp != nil {
		appMap[baseApp.GetName()] = baseApp
	}
	if baseNode != nil {
		nodeMap[baseNode.Name] = baseNode
	}
	units := make([]provision.Unit, len(pods))
	for i, pod := range pods {
		l := labelSetFromMeta(&pod.ObjectMeta)
		node, ok := nodeMap[pod.Spec.NodeName]
		if !ok {
			node, err = client.Core().Nodes().Get(pod.Spec.NodeName, metav1.GetOptions{})
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
		units[i] = provision.Unit{
			ID:          pod.Name,
			Name:        pod.Name,
			AppName:     l.AppName(),
			ProcessName: appProcess,
			Type:        l.AppPlatform(),
			IP:          tsuruNet.URLToHost(wrapper.Address()),
			Status:      stateMap[pod.Status.Phase],
			Address:     url,
		}
	}
	return units, nil
}

func (p *kubernetesProvisioner) Units(a provision.App) ([]provision.Unit, error) {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return nil, err
	}
	kubeConf := getKubeConfig()
	err = client.SetTimeout(kubeConf.APIShortTimeout)
	if err != nil {
		return nil, err
	}
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
	pods, err := client.Core().Pods(client.Namespace()).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(l.ToAppSelector())).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return p.podsToUnits(client, pods.Items, a, nil)
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
	nodes, err := client.Core().Nodes().List(metav1.ListOptions{
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
	pod, err := client.Core().Pods(client.Namespace()).Get(unitID, metav1.GetOptions{})
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
	err := forEachCluster(func(c *clusterClient) error {
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

func (p *kubernetesProvisioner) listNodesForCluster(cluster *clusterClient, addressFilter []string) ([]provision.Node, error) {
	var nodes []provision.Node
	var addressSet set.Set
	if len(addressFilter) > 0 {
		addressSet = set.FromSlice(addressFilter)
	}
	nodeList, err := cluster.Core().Nodes().List(metav1.ListOptions{})
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

func setNodeMetadata(node *apiv1.Node, pool string, meta map[string]string) {
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
	if pool != "" {
		baseNodeLabels := provision.NodeLabels(provision.NodeLabelsOpts{
			Pool:   pool,
			Prefix: tsuruLabelPrefix,
		})
		for k, v := range baseNodeLabels.ToLabels() {
			delete(node.Annotations, k)
			node.Labels[k] = v
		}
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
	setNodeMetadata(node, opts.Pool, opts.Metadata)
	_, err = client.Core().Nodes().Create(node)
	if k8sErrors.IsAlreadyExists(err) {
		return p.UpdateNode(provision.UpdateNodeOptions{
			Address:  hostAddr,
			Metadata: opts.Metadata,
			Pool:     opts.Pool,
		})
	}
	if err == nil {
		go refreshNodeTaints(client, hostAddr)
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
		_, err = client.Core().Nodes().Update(node)
		if err != nil {
			return errors.WithStack(err)
		}
		var pods []apiv1.Pod
		pods, err = podsFromNode(client, node.Name)
		if err != nil {
			return err
		}
		for _, pod := range pods {
			err = client.Core().Pods(client.Namespace()).Evict(&policy.Eviction{
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
	err = client.Core().Nodes().Delete(node.Name, &metav1.DeleteOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (p *kubernetesProvisioner) NodeForNodeData(nodeData provision.NodeStatusData) (provision.Node, error) {
	return provision.FindNodeByAddrs(p, nodeData.Addrs)
}

func (p *kubernetesProvisioner) findNodeByAddress(address string) (*clusterClient, *kubernetesNodeWrapper, error) {
	var (
		foundNode    *kubernetesNodeWrapper
		foundCluster *clusterClient
	)
	err := forEachCluster(func(c *clusterClient) error {
		if foundNode != nil {
			return nil
		}
		nodeList, err := c.Core().Nodes().List(metav1.ListOptions{})
		if err != nil {
			return err
		}
		for i := range nodeList.Items {
			nodeWrapper := &kubernetesNodeWrapper{
				node:    &nodeList.Items[i],
				prov:    p,
				cluster: c,
			}
			if address == nodeWrapper.node.Name ||
				address == nodeWrapper.Address() {
				foundNode = nodeWrapper
				foundCluster = c
				break
			}
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
	client, nodeWrapper, err := p.findNodeByAddress(opts.Address)
	if err != nil {
		return err
	}
	node := nodeWrapper.node
	if opts.Disable {
		node.Spec.Unschedulable = true
	} else if opts.Enable {
		node.Spec.Unschedulable = false
	}
	setNodeMetadata(node, opts.Pool, opts.Metadata)
	_, err = client.Core().Nodes().Update(node)
	if err == nil {
		go refreshNodeTaints(client, opts.Address)
	}
	return err
}

func (p *kubernetesProvisioner) ImageDeploy(a provision.App, imageID string, evt *event.Event) (string, error) {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return "", err
	}
	if !strings.Contains(imageID, ":") {
		imageID = fmt.Sprintf("%s:latest", imageID)
	}
	fmt.Fprintln(evt, "---- Pulling image to tsuru ----")
	newImage, err := image.AppNewImageName(a.GetName())
	if err != nil {
		return "", err
	}
	imageInspect, err := imageTagAndPush(client, a, imageID, newImage)
	if err != nil {
		return "", err
	}
	if len(imageInspect.Config.ExposedPorts) > 1 {
		return "", errors.Errorf("too many ports exposed in Dockerfile, only one allowed: %+v", imageInspect.Config.ExposedPorts)
	}
	procfileRaw, err := procfileInspectPod(client, a, imageID)
	if err != nil {
		return "", err
	}
	procfile := image.GetProcessesFromProcfile(procfileRaw)
	if len(procfile) == 0 {
		fmt.Fprintln(evt, " ---> Procfile not found, using entrypoint and cmd")
		cmds := append(imageInspect.Config.Entrypoint, imageInspect.Config.Cmd...)
		if len(cmds) == 0 {
			return "", errors.New("neither Procfile nor entrypoint and cmd set")
		}
		procfile["web"] = cmds
	}
	for k, v := range procfile {
		fmt.Fprintf(evt, " ---> Process %q found with commands: %q\n", k, v)
	}
	imageData := image.ImageMetadata{
		Name:      newImage,
		Processes: procfile,
	}
	for k := range imageInspect.Config.ExposedPorts {
		imageData.ExposedPort = k
	}
	err = imageData.Save()
	if err != nil {
		return "", err
	}
	manager := &serviceManager{
		client: client,
		writer: evt,
	}
	err = servicecommon.RunServicePipeline(manager, a, newImage, nil)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return newImage, nil
}

func (p *kubernetesProvisioner) UploadDeploy(a provision.App, archiveFile io.ReadCloser, fileSize int64, build bool, evt *event.Event) (string, error) {
	defer archiveFile.Close()
	if build {
		return "", errors.New("running UploadDeploy with build=true is not yet supported")
	}
	baseImage := image.GetBuildImage(a)
	buildingImage, err := image.AppNewImageName(a.GetName())
	if err != nil {
		return "", errors.WithStack(err)
	}
	deployPodName, err := deployPodNameForApp(a)
	if err != nil {
		return "", err
	}
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return "", err
	}
	defer cleanupPod(client, deployPodName)
	params := buildPodParams{
		app:              a,
		client:           client,
		sourceImage:      baseImage,
		destinationImage: buildingImage,
		attachInput:      archiveFile,
		attachOutput:     evt,
	}
	err = createBuildPod(params)
	if err != nil {
		return "", err
	}
	manager := &serviceManager{
		client: client,
		writer: evt,
	}
	err = servicecommon.RunServicePipeline(manager, a, buildingImage, nil)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return buildingImage, nil
}

func (p *kubernetesProvisioner) UpgradeNodeContainer(name string, pool string, writer io.Writer) error {
	m := nodeContainerManager{}
	return servicecommon.UpgradeNodeContainer(&m, name, pool, writer)
}

func (p *kubernetesProvisioner) RemoveNodeContainer(name string, pool string, writer io.Writer) error {
	err := forEachCluster(func(cluster *clusterClient) error {
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
	pods, err := client.Core().Pods(client.Namespace()).List(metav1.ListOptions{
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

func runIsolatedCmdPod(client *clusterClient, a provision.App, out io.Writer, cmds []string) error {
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
		labels: labels,
		cmds:   cmds,
		envs:   envs,
		name:   baseName,
		image:  imgName,
		pool:   a.GetPool(),
	})
}

func (p *kubernetesProvisioner) ExecuteCommandIsolated(stdout, stderr io.Writer, a provision.App, cmd string, args ...string) error {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return err
	}
	cmds := append([]string{"/bin/sh", "-c", cmd}, args...)
	return runIsolatedCmdPod(client, a, stdout, cmds)
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
