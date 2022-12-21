// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/provision/dockercommon"
	_ "github.com/tsuru/tsuru/provision/kubernetes/authplugin/gcpwithproxy" // import custom authplugin that have proxy support
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	"github.com/tsuru/tsuru/provision/node"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	provTypes "github.com/tsuru/tsuru/types/provision"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // gcp default auth plugin
	"k8s.io/client-go/tools/remotecommand"
)

const (
	provisionerName                            = "kubernetes"
	defaultKubeAPITimeout                      = time.Minute
	defaultPodReadyTimeout                     = time.Minute
	defaultPodRunningTimeout                   = 10 * time.Minute
	defaultDeploymentProgressTimeout           = 10 * time.Minute
	defaultAttachTimeoutAfterContainerFinished = time.Minute
	defaultSidecarImageName                    = "tsuru/deploy-agent:0.10.2"
	defaultPreStopSleepSeconds                 = 10
)

var (
	defaultEphemeralStorageLimit = resource.MustParse("100Mi")
	podAllowedReasonsToFail      = map[string]bool{
		"shutdown":     true,
		"evicted":      true,
		"nodeaffinity": true,
		"terminated":   true,
	}
)

type kubernetesProvisioner struct {
	mu                 sync.Mutex
	clusterControllers map[string]*clusterController
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
	_ provision.InterAppProvisioner      = &kubernetesProvisioner{}
	_ provision.HCProvisioner            = &kubernetesProvisioner{}
	_ provision.VersionsProvisioner      = &kubernetesProvisioner{}
	_ provision.LogsProvisioner          = &kubernetesProvisioner{}
	_ provision.MetricsProvisioner       = &kubernetesProvisioner{}
	_ provision.AutoScaleProvisioner     = &kubernetesProvisioner{}
	_ cluster.ClusteredProvisioner       = &kubernetesProvisioner{}
	_ provision.UpdatableProvisioner     = &kubernetesProvisioner{}
	_ provision.MultiRegistryProvisioner = &kubernetesProvisioner{}
	_ provision.KillUnitProvisioner      = &kubernetesProvisioner{}

	mainKubernetesProvisioner *kubernetesProvisioner
)

func init() {
	mainKubernetesProvisioner = &kubernetesProvisioner{
		clusterControllers: map[string]*clusterController{},
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
	deploySidecarImage string
	deployInspectImage string
	APITimeout         time.Duration
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
	// RegisterNode if set will make tsuru add a node object to the kubernetes
	// API. Otherwise tsuru will expect the node to be already registered.
	RegisterNode bool
}

func getKubeConfig() kubernetesConfig {
	conf := kubernetesConfig{}
	conf.LogLevel, _ = config.GetInt("kubernetes:log-level")
	conf.deploySidecarImage, _ = config.GetString("kubernetes:deploy-sidecar-image")
	if conf.deploySidecarImage == "" {
		conf.deploySidecarImage = defaultSidecarImageName
	}
	conf.deployInspectImage, _ = config.GetString("kubernetes:deploy-inspect-image")
	if conf.deployInspectImage == "" {
		conf.deployInspectImage = defaultSidecarImageName
	}
	apiTimeout, _ := config.GetFloat("kubernetes:api-timeout")
	if apiTimeout != 0 {
		conf.APITimeout = time.Duration(apiTimeout * float64(time.Second))
	} else {
		conf.APITimeout = defaultKubeAPITimeout
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
	conf.RegisterNode, _ = config.GetBool("kubernetes:register-node")
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

	initLocalCluster()

	err := initAllControllers(p)
	if err == provTypes.ErrNoCluster {
		return nil
	}
	return err
}

func initLocalCluster() {
	ctx := context.Background()
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" || os.Getenv("KUBERNETES_SERVICE_PORT") == "" {
		return // not running inside a kubernetes cluster
	}

	log.Debugf("[kubernetes-provisioner] tsuru is running inside a kubernetes cluster")

	clusters, err := servicemanager.Cluster.List(ctx)
	if err != nil && err != provTypes.ErrNoCluster {
		log.Errorf("[kubernetes-provisioner] could not list clusters: %s", err.Error())
		return
	}

	if len(clusters) > 0 {
		return
	}

	log.Debugf("[kubernetes-provisioner] no kubernetes clusters found, adding default")

	err = servicemanager.Cluster.Create(ctx, provTypes.Cluster{
		Name:        "local",
		Default:     true,
		Local:       true,
		Provisioner: provisionerName,
		CustomData: map[string]string{
			enableLogsFromAPIServerKey:    "true",
			disableDefaultNodeSelectorKey: "true",
			disableUnitRegisterCmdKey:     "true",
			disableNodeContainers:         "true",
		},
	})

	if err != nil {
		log.Errorf("[kubernetes-provisioner] could not create default cluster: %v", err)
	}

	pools, err := servicemanager.Pool.List(ctx)
	if err != nil {
		log.Errorf("[kubernetes-provisioner] could not list pools: %v", err)
	}

	if len(pools) > 0 {
		return
	}

	log.Debugf("[kubernetes-provisioner] no pool found, adding default")

	err = pool.AddPool(ctx, pool.AddPoolOptions{
		Name:        "local",
		Provisioner: provisionerName,
		Default:     true,
	})

	if err != nil {
		log.Errorf("[kubernetes-provisioner] could not create default pool: %v", err)
	}
}

func (p *kubernetesProvisioner) InitializeCluster(ctx context.Context, c *provTypes.Cluster) error {
	clusterClient, err := NewClusterClient(c)
	if err != nil {
		return err
	}
	stopClusterController(ctx, p, clusterClient)
	_, err = getClusterController(p, clusterClient)
	return err
}

func (p *kubernetesProvisioner) ValidateCluster(c *provTypes.Cluster) error {
	multiErrors := tsuruErrors.NewMultiError()

	if _, ok := c.CustomData[singlePoolKey]; ok && len(c.Pools) != 1 {
		multiErrors.Add(errors.Errorf("only one pool is allowed to use entire cluster as single-pool. %d pools found", len(c.Pools)))
	}

	if c.KubeConfig != nil {
		if len(c.Addresses) > 1 {
			multiErrors.Add(errors.New("when kubeConfig is set the use of addresses is not used"))
		}
		if c.CaCert != nil {
			multiErrors.Add(errors.New("when kubeConfig is set the use of cacert is not used"))
		}
		if c.ClientCert != nil {
			multiErrors.Add(errors.New("when kubeConfig is set the use of clientcert is not used"))
		}
		if c.ClientKey != nil {
			multiErrors.Add(errors.New("when kubeConfig is set the use of clientkey is not used"))
		}
		if c.KubeConfig.Cluster.Server == "" {
			multiErrors.Add(errors.New("kubeConfig.cluster.server field is required"))
		}
	}

	return multiErrors.ToError()
}

func (p *kubernetesProvisioner) ClusterHelp() provTypes.ClusterHelpInfo {
	return provTypes.ClusterHelpInfo{
		CustomDataHelp:  clusterHelp,
		ProvisionerHelp: "Represents a kubernetes cluster, the address parameter must point to a valid kubernetes apiserver endpoint.",
	}
}

func (p *kubernetesProvisioner) DeleteCluster(ctx context.Context, c *provTypes.Cluster) error {
	stopClusterControllerByName(ctx, p, c.Name)
	return nil
}

func (p *kubernetesProvisioner) GetName() string {
	return provisionerName
}

func (p *kubernetesProvisioner) Provision(ctx context.Context, a provision.App) error {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return err
	}
	return ensureAppCustomResourceSynced(ctx, client, a)
}

func (p *kubernetesProvisioner) Destroy(ctx context.Context, a provision.App) error {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return err
	}
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	app, err := tclient.TsuruV1().Apps(client.Namespace()).Get(ctx, a.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := p.removeResources(ctx, client, app, a); err != nil {
		return err
	}
	return tclient.TsuruV1().Apps(client.Namespace()).Delete(ctx, a.GetName(), metav1.DeleteOptions{})
}

func (p *kubernetesProvisioner) DestroyVersion(ctx context.Context, a provision.App, version appTypes.AppVersion) error {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return err
	}
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	app, err := tclient.TsuruV1().Apps(client.Namespace()).Get(ctx, a.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}
	return p.removeResourcesFromVersion(ctx, client, app, a, version)
}

func (p *kubernetesProvisioner) removeResourcesFromVersion(ctx context.Context, client *ClusterClient, tsuruApp *tsuruv1.App, app provision.App, version appTypes.AppVersion) error {
	allProcesses, err := version.Processes()
	if err != nil {
		return err
	}
	processes := []string{}
	for processName := range allProcesses {
		processes = append(processes, processName)
	}

	depList := []*appsv1.Deployment{}
	svcList := []apiv1.Service{}
	for _, process := range processes {
		var dep *appsv1.Deployment
		dep, err = deploymentForVersion(ctx, client, app, process, version.Version())
		if err != nil {
			return err
		}
		depList = append(depList, dep)

		var svcs []apiv1.Service
		svcs, err = allServicesForAppProcess(ctx, client, app, process)
		if err != nil && !k8sErrors.IsNotFound(err) {
			return err
		}

		for _, svc := range svcs {
			labels := labelSetFromMeta(&svc.ObjectMeta)
			svcVersion := labels.AppVersion()
			if svcVersion == version.Version() {
				svcList = append(svcList, svc)
			}
		}
	}

	multiErrors := tsuruErrors.NewMultiError()
	for _, dd := range depList {
		err = cleanupSingleDeployment(ctx, client, dd)
		if err != nil {
			multiErrors.Add(err)
		}
	}

	for _, ss := range svcList {
		err = client.CoreV1().Services(tsuruApp.Spec.NamespaceName).Delete(ctx, ss.Name, metav1.DeleteOptions{
			PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
		})
		if err != nil && !k8sErrors.IsNotFound(err) {
			multiErrors.Add(errors.WithStack(err))
		}
	}

	for _, process := range processes {
		if err := p.deleteHPAByVersionAndProcess(ctx, app, process, version.Version()); err != nil {
			multiErrors.Add(err)
		}

		if err := deleteVPAsByVersion(ctx, client, app, version.Version()); err != nil {
			multiErrors.Add(err)
		}
	}

	return multiErrors.ToError()
}

func (p *kubernetesProvisioner) removeResources(ctx context.Context, client *ClusterClient, tsuruApp *tsuruv1.App, app provision.App) error {
	deps, err := allDeploymentsForAppNS(ctx, client, tsuruApp.Spec.NamespaceName, app)
	if err != nil {
		return err
	}
	svcs, err := allServicesForAppNS(ctx, client, tsuruApp.Spec.NamespaceName, app)
	if err != nil {
		return err
	}
	multiErrors := tsuruErrors.NewMultiError()
	for _, dd := range deps {
		err = cleanupSingleDeployment(ctx, client, &dd)
		if err != nil {
			multiErrors.Add(err)
		}
	}
	for _, ss := range svcs {
		err = client.CoreV1().Services(tsuruApp.Spec.NamespaceName).Delete(ctx, ss.Name, metav1.DeleteOptions{
			PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
		})
		if err != nil && !k8sErrors.IsNotFound(err) {
			multiErrors.Add(errors.WithStack(err))
		}
	}
	vols, err := servicemanager.Volume.ListByApp(ctx, app.GetName())
	if err != nil {
		multiErrors.Add(errors.WithStack(err))
	} else {
		for _, vol := range vols {
			vol.Binds, err = servicemanager.Volume.Binds(ctx, &vol)
			if err != nil {
				continue
			}

			bindedToOtherApps := false
			for _, b := range vol.Binds {
				if b.ID.App != app.GetName() {
					bindedToOtherApps = true
					break
				}
			}
			if !bindedToOtherApps {
				err = deleteVolume(ctx, client, vol.Name)
				if err != nil {
					multiErrors.Add(errors.WithStack(err))
				}
			}
		}
	}
	if err = removeAllPDBs(ctx, client, app); err != nil {
		multiErrors.Add(errors.WithStack(err))
	}
	err = client.CoreV1().ServiceAccounts(tsuruApp.Spec.NamespaceName).Delete(ctx, tsuruApp.Spec.ServiceAccountName, metav1.DeleteOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(errors.WithStack(err))
	}
	err = p.deleteAllAutoScale(ctx, app)
	if err != nil {
		multiErrors.Add(err)
	}
	err = deleteAllVPA(ctx, client, app)
	if err != nil {
		multiErrors.Add(err)
	}
	err = deleteAllBackendConfig(ctx, client, app)
	if err != nil {
		multiErrors.Add(err)
	}
	return multiErrors.ToError()
}

func versionsForAppProcess(ctx context.Context, client *ClusterClient, a provision.App, process string, ignoreBaseDepIfStopped bool) ([]appTypes.AppVersion, error) {
	grouped, err := deploymentsDataForApp(ctx, client, a)
	if err != nil {
		return nil, err
	}

	if ignoreBaseDepIfStopped {
		ignoreBaseDep(grouped.versioned)
	}

	versionSet := map[int]struct{}{}
	for v, deps := range grouped.versioned {
		for _, depData := range deps {
			if process == "" || process == depData.process {
				versionSet[v] = struct{}{}
			}
		}
	}

	var versions []appTypes.AppVersion
	for v := range versionSet {
		version, err := servicemanager.AppVersion.VersionByImageOrVersion(ctx, a, strconv.Itoa(v))
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func changeState(ctx context.Context, a provision.App, process string, version appTypes.AppVersion, state servicecommon.ProcessState, w io.Writer) error {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return err
	}
	err = ensureAppCustomResourceSynced(ctx, client, a)
	if err != nil {
		return err
	}

	var versions []appTypes.AppVersion
	if version == nil {
		versions, err = versionsForAppProcess(ctx, client, a, process, true)
		if err != nil {
			return err
		}
	} else {
		versions = append(versions, version)
	}

	if len(versions) == 0 {
		version, err = servicemanager.AppVersion.LatestSuccessfulVersion(ctx, a)
		if err != nil {
			return err
		}

		versions = append(versions, version)
	}

	var multiErr tsuruErrors.MultiError
	for _, v := range versions {
		err = servicecommon.ChangeAppState(ctx, &serviceManager{
			client: client,
			writer: w,
		}, a, process, state, v)
		if err != nil {
			multiErr.Add(errors.Wrapf(err, "unable to update version v%d", v.Version()))
		}
	}
	return multiErr.ToError()
}

func patchDeployment(ctx context.Context, client *ClusterClient, a provision.App, patchType types.PatchType, patch []byte, dep *appsv1.Deployment, version appTypes.AppVersion, w io.Writer, process string) error {
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}
	newDep, err := client.AppsV1().Deployments(ns).Patch(ctx, dep.Name, patchType, patch, metav1.PatchOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	events, err := client.CoreV1().Events(ns).List(ctx, listOptsForResourceEvent("Pod", ""))
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = monitorDeployment(ctx, client, newDep, a, process, w, events.ResourceVersion, version)
	if err != nil {
		if _, ok := err.(provision.ErrUnitStartup); ok {
			return err
		}
		return provision.ErrUnitStartup{Err: err}
	}
	sm := &serviceManager{
		client: client,
		writer: w,
	}
	return sm.CleanupServices(ctx, a, version.Version(), true)
}

func ensureProcessName(processName string, version appTypes.AppVersion) (string, error) {
	if processName == "" {
		cmdData, err := dockercommon.ContainerCmdsDataFromVersion(version)
		if err != nil {
			return "", err
		}
		_, processName, err = dockercommon.ProcessCmdForVersion(processName, cmdData)
		if err != nil {
			return "", errors.WithStack(err)
		}
	}
	return processName, nil
}

func changeUnits(ctx context.Context, a provision.App, units int, processName string, version appTypes.AppVersion, w io.Writer) error {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return err
	}
	err = ensureAppCustomResourceSynced(ctx, client, a)
	if err != nil {
		return err
	}
	processName, err = ensureProcessName(processName, version)
	if err != nil {
		return err
	}
	dep, err := deploymentForVersion(ctx, client, a, processName, version.Version())
	if err != nil && !k8sErrors.IsNotFound(err) {
		return err
	}
	if dep == nil {
		return servicecommon.ChangeUnits(ctx, &serviceManager{
			client: client,
			writer: w,
		}, a, units, processName, version)
	}
	zero := int32(0)
	if dep.Spec.Replicas == nil {
		dep.Spec.Replicas = &zero
	}
	if w == nil {
		w = ioutil.Discard
	}
	newReplicas := int(*dep.Spec.Replicas) + units
	if newReplicas <= 0 {
		fmt.Fprintf(w, "---- Calling app stop internally as the number of units is zero ----\n")
		return GetProvisioner().Stop(ctx, a, processName, version, w)
	}
	patchType, patch, err := replicasPatch(newReplicas, processName)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "---- Patching from %d to %d units ----\n", *dep.Spec.Replicas, newReplicas)
	return patchDeployment(ctx, client, a, patchType, patch, dep, version, w, processName)
}

func replicasPatch(replicas int, process string) (types.PatchType, []byte, error) {
	patch, err := json.Marshal([]interface{}{
		map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/replicas",
			"value": replicas,
		},
	})
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	return types.JSONPatchType, patch, nil
}

func (p *kubernetesProvisioner) AddUnits(ctx context.Context, a provision.App, units uint, processName string, version appTypes.AppVersion, w io.Writer) error {
	return changeUnits(ctx, a, int(units), processName, version, w)
}

func (p *kubernetesProvisioner) RemoveUnits(ctx context.Context, a provision.App, units uint, processName string, version appTypes.AppVersion, w io.Writer) error {
	return changeUnits(ctx, a, -int(units), processName, version, w)
}

func (p *kubernetesProvisioner) Restart(ctx context.Context, a provision.App, process string, version appTypes.AppVersion, w io.Writer) error {
	return changeState(ctx, a, process, version, servicecommon.ProcessState{Start: true, Restart: true}, w)
}

func (p *kubernetesProvisioner) Start(ctx context.Context, a provision.App, process string, version appTypes.AppVersion, w io.Writer) error {
	return changeState(ctx, a, process, version, servicecommon.ProcessState{Start: true}, w)
}

func (p *kubernetesProvisioner) Stop(ctx context.Context, a provision.App, process string, version appTypes.AppVersion, w io.Writer) error {
	return changeState(ctx, a, process, version, servicecommon.ProcessState{Stop: true}, w)
}

func (p *kubernetesProvisioner) Sleep(ctx context.Context, a provision.App, process string, version appTypes.AppVersion) error {
	return changeState(ctx, a, process, version, servicecommon.ProcessState{Stop: true, Sleep: true}, nil)
}

var stateMap = map[apiv1.PodPhase]provision.Status{
	apiv1.PodPending:   provision.StatusCreated,
	apiv1.PodRunning:   provision.StatusStarted,
	apiv1.PodSucceeded: provision.StatusStopped,
	apiv1.PodFailed:    provision.StatusError,
	apiv1.PodUnknown:   provision.StatusError,
}

func (p *kubernetesProvisioner) podsToUnits(ctx context.Context, client *ClusterClient, pods []apiv1.Pod, baseApp provision.App) ([]provision.Unit, error) {
	var apps []provision.App
	if baseApp != nil {
		apps = append(apps, baseApp)
	}
	return p.podsToUnitsMultiple(ctx, client, pods, apps)
}

func (p *kubernetesProvisioner) podsToUnitsMultiple(ctx context.Context, client *ClusterClient, pods []apiv1.Pod, baseApps []provision.App) ([]provision.Unit, error) {
	var err error
	if len(pods) == 0 {
		return nil, nil
	}
	appMap := map[string]provision.App{}
	portsMap := map[string][]int32{}
	for _, baseApp := range baseApps {
		appMap[baseApp.GetName()] = baseApp
	}
	controller, err := getClusterController(p, client)
	if err != nil {
		return nil, err
	}
	svcInformer, err := controller.getServiceInformer()
	if err != nil {
		return nil, err
	}
	var units []provision.Unit
	for _, pod := range pods {
		if isTerminating(pod) || podIsAllowedToFail(pod) {
			continue
		}
		l := labelSetFromMeta(&pod.ObjectMeta)
		podApp, ok := appMap[l.AppName()]
		if !ok {
			podApp, err = app.GetByName(ctx, l.AppName())
			if err != nil {
				return nil, errors.WithStack(err)
			}
			appMap[podApp.GetName()] = podApp
		}
		u := &url.URL{
			Scheme: "http",
			Host:   pod.Status.HostIP,
		}
		urls := []url.URL{}
		appProcess := l.AppProcess()
		appVersion := l.AppVersion()
		isRoutable := l.IsRoutable()
		if appVersion == 0 {
			isRoutable = true
			if len(pod.Spec.Containers) > 0 {
				_, tag := image.SplitImageName(pod.Spec.Containers[0].Image)
				appVersion, _ = strconv.Atoi(strings.TrimPrefix(tag, "v"))
			}
		}
		if appProcess != "" {
			var srvName string
			if isRoutable {
				srvName = serviceNameForAppBase(podApp, appProcess)
			} else {
				srvName = serviceNameForApp(podApp, appProcess, appVersion)
			}
			ports, ok := portsMap[srvName]
			if !ok {
				ports, err = getServicePorts(svcInformer, srvName, pod.ObjectMeta.Namespace)
				if err != nil {
					return nil, err
				}
				portsMap[srvName] = ports
			}
			if len(ports) > 0 {
				u.Host = fmt.Sprintf("%s:%d", u.Host, ports[0])
				for _, p := range ports {
					urls = append(urls, url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", pod.Status.HostIP, p)})
				}
			}
		}

		var status provision.Status
		var reason string
		if pod.Status.Phase == apiv1.PodRunning {
			status, reason = extractStatusFromContainerStatuses(pod.Status.ContainerStatuses)
		} else {
			status = stateMap[pod.Status.Phase]
			reason = pod.Status.Reason
		}

		createdAt := pod.CreationTimestamp.Time.In(time.UTC)
		units = append(units, provision.Unit{
			ID:           pod.Name,
			Name:         pod.Name,
			AppName:      l.AppName(),
			ProcessName:  appProcess,
			Type:         l.AppPlatform(),
			IP:           pod.Status.HostIP,
			Status:       status,
			StatusReason: reason,
			Address:      u,
			Addresses:    urls,
			Version:      appVersion,
			Routable:     isRoutable,
			Restarts:     containersRestarts(pod.Status.ContainerStatuses),
			CreatedAt:    &createdAt,
			Ready:        containersReady(pod.Status.ContainerStatuses),
		})
	}
	return units, nil
}

func containersRestarts(containersStatus []apiv1.ContainerStatus) *int32 {
	restarts := int32(0)
	for _, containerStatus := range containersStatus {
		restarts += containerStatus.RestartCount
	}
	return &restarts
}

func containersReady(containersStatus []apiv1.ContainerStatus) *bool {
	ready := len(containersStatus) > 0
	for _, containerStatus := range containersStatus {
		if !containerStatus.Ready {
			ready = false
			break
		}
	}
	return &ready
}

func extractStatusFromContainerStatuses(statuses []apiv1.ContainerStatus) (provision.Status, string) {
	for _, containerStatus := range statuses {
		if containerStatus.Ready {
			continue
		}

		if containerStatus.State.Waiting != nil {
			return provision.StatusError, containerStatus.State.Waiting.Reason
		}

		if containerStatus.LastTerminationState.Terminated != nil {
			return provision.StatusError, containerStatus.LastTerminationState.Terminated.Reason
		}

		return provision.StatusStarting, ""
	}
	return provision.StatusStarted, ""
}

// merged from https://github.com/kubernetes/kubernetes/blob/1f69c34478800e150acd022f6313a15e1cb7a97c/pkg/quota/evaluator/core/pods.go#L333
// and https://github.com/kubernetes/kubernetes/blob/560e15fb9acee4b8391afbc21fc3aea7b771e2c4/pkg/printers/internalversion/printers.go#L606
func isTerminating(pod apiv1.Pod) bool {
	return pod.Spec.ActiveDeadlineSeconds != nil && *pod.Spec.ActiveDeadlineSeconds >= int64(0) || pod.DeletionTimestamp != nil
}

func podIsAllowedToFail(pod apiv1.Pod) bool {
	reason := strings.ToLower(pod.Status.Reason)
	return pod.Status.Phase == apiv1.PodFailed && podAllowedReasonsToFail[reason]
}

func (p *kubernetesProvisioner) Units(ctx context.Context, apps ...provision.App) ([]provision.Unit, error) {
	cApps, err := clustersForApps(ctx, apps)
	if err != nil {
		return nil, err
	}
	var units []provision.Unit
	for _, cApp := range cApps {
		pods, err := p.podsForApps(ctx, cApp.client, cApp.apps)
		if err != nil {
			return nil, err
		}
		clusterUnits, err := p.podsToUnitsMultiple(ctx, cApp.client, pods, cApp.apps)
		if err != nil {
			return nil, err
		}
		units = append(units, clusterUnits...)
	}
	return units, nil
}

func (p *kubernetesProvisioner) podsForApps(ctx context.Context, client *ClusterClient, apps []provision.App) ([]apiv1.Pod, error) {
	inSelectorMap := map[string][]string{}
	for _, a := range apps {
		l, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
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
	controller, err := getClusterController(p, client)
	if err != nil {
		return nil, err
	}
	informer, err := controller.getPodInformer()
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

func (p *kubernetesProvisioner) RoutableAddresses(ctx context.Context, a provision.App) ([]appTypes.RoutableAddresses, error) {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return nil, err
	}
	controller, err := getClusterController(p, client)
	if err != nil {
		return nil, err
	}
	svcInformer, err := controller.getServiceInformer()
	if err != nil {
		return nil, err
	}
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}

	svcs, err := allServicesForAppInformer(ctx, svcInformer, ns, a)
	if err != nil {
		return nil, err
	}

	processSet := set.Set{}
	for _, svc := range svcs {
		ls := labelOnlySetFromMeta(&svc.ObjectMeta)
		if ls.IsRoutable() {
			processSet.Add(ls.AppProcess())
		}
	}
	webProcessName := provision.MainAppProcess(processSet.ToList())

	var allAddrs []appTypes.RoutableAddresses
	for _, svc := range svcs {
		ls := labelOnlySetFromMeta(&svc.ObjectMeta)

		if ls.IsHeadlessService() {
			continue
		}

		processName := ls.AppProcess()
		version := ls.AppVersion()

		var rAddr appTypes.RoutableAddresses

		if processName == webProcessName {
			var prefix string
			if version != 0 {
				prefix = fmt.Sprintf("v%d.version", version)
			}
			rAddr, err = p.routableAddrForProcess(ctx, client, a, processName, prefix, version, svc)
			if err != nil {
				return nil, err
			}
			allAddrs = append(allAddrs, rAddr)
		}

		var prefix string
		if version == 0 {
			prefix = fmt.Sprintf("%s.process", processName)
		} else {
			prefix = fmt.Sprintf("v%d.version.%s.process", version, processName)
		}
		rAddr, err = p.routableAddrForProcess(ctx, client, a, processName, prefix, version, svc)
		if err != nil {
			return nil, err
		}
		allAddrs = append(allAddrs, rAddr)

	}
	return allAddrs, nil
}

func (p *kubernetesProvisioner) routableAddrForProcess(ctx context.Context, client *ClusterClient, a provision.App, processName, prefix string, version int, svc apiv1.Service) (appTypes.RoutableAddresses, error) {
	routableAddrs := appTypes.RoutableAddresses{
		Prefix: prefix,
		ExtraData: map[string]string{
			"service":   svc.Name,
			"namespace": svc.Namespace,
		},
	}
	var pubPort int32
	if len(svc.Spec.Ports) > 0 {
		pubPort = svc.Spec.Ports[0].NodePort
	}
	if pubPort == 0 {
		return routableAddrs, nil
	}
	addrs, err := p.addressesForApp(ctx, client, a, processName, pubPort, version)
	if err != nil || addrs == nil {
		return routableAddrs, err
	}
	routableAddrs.Addresses = addrs
	return routableAddrs, nil
}

func (p *kubernetesProvisioner) addressesForApp(ctx context.Context, client *ClusterClient, a provision.App, processName string, pubPort int32, version int) ([]*url.URL, error) {
	pods, err := p.podsForApps(ctx, client, []provision.App{a})
	if err != nil {
		return nil, err
	}
	addrs := make([]*url.URL, 0)
	for _, pod := range pods {
		labelSet := labelSetFromMeta(&pod.ObjectMeta)
		if labelSet.IsIsolatedRun() {
			continue
		}
		if labelSet.AppProcess() != processName {
			continue
		}
		if version != 0 && labelSet.AppVersion() != version {
			continue
		}
		if isPodReady(&pod) {
			addrs = append(addrs, &url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s:%d", pod.Status.HostIP, pubPort),
			})
		}
	}
	return addrs, nil
}

func (p *kubernetesProvisioner) RegisterUnit(ctx context.Context, a provision.App, unitID string, customData map[string]interface{}) error {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return err
	}
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}
	pod, err := client.CoreV1().Pods(ns).Get(ctx, unitID, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return &provision.UnitNotFoundError{ID: unitID}
		}
		return errors.WithStack(err)
	}
	units, err := p.podsToUnits(ctx, client, []apiv1.Pod{*pod}, a)
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
	version, err := servicemanager.AppVersion.VersionByPendingImage(ctx, a, buildingImage)
	if err != nil {
		return errors.WithStack(err)
	}
	err = version.AddData(appTypes.AddVersionDataArgs{
		CustomData: customData,
	})
	return errors.WithStack(err)
}

func (p *kubernetesProvisioner) ListNodes(ctx context.Context, addressFilter []string) ([]provision.Node, error) {
	var nodes []provision.Node
	err := forEachCluster(ctx, func(c *ClusterClient) error {
		clusterNodes, err := p.listNodesForCluster(c, nodeFilter{addresses: addressFilter})
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

func (p *kubernetesProvisioner) InternalAddresses(ctx context.Context, a provision.App) ([]provision.AppInternalAddress, error) {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return nil, err
	}
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}

	controller, err := getClusterController(p, client)
	if err != nil {
		return nil, err
	}
	svcInformer, err := controller.getServiceInformer()
	if err != nil {
		return nil, err
	}

	svcs, err := allServicesForAppInformer(ctx, svcInformer, ns, a)
	if err != nil {
		return nil, err
	}

	sort.Slice(svcs, func(i, j int) (x bool) {
		iVersion := svcs[i].ObjectMeta.Labels[tsuruLabelAppVersion]
		jVersion := svcs[j].ObjectMeta.Labels[tsuruLabelAppVersion]
		iProcess := svcs[i].ObjectMeta.Labels[tsuruLabelAppProcess]
		jProcess := svcs[j].ObjectMeta.Labels[tsuruLabelAppProcess]

		// we priorize the web process without versioning
		// in the most cases will be address used to bind related services
		// the list of services will send to tsuru services, then they uses the first address to automatic bind
		if iProcess == provision.WebProcessName && iVersion == "" {
			return true
		} else if jProcess == provision.WebProcessName && jVersion == "" {
			return false
		}

		if iVersion != jVersion {
			return iVersion < jVersion
		}

		return iProcess < jProcess
	})

	addresses := []provision.AppInternalAddress{}
	for _, service := range svcs {
		// we can't show headless services
		if service.Spec.ClusterIP == "None" {
			continue
		}
		for _, port := range service.Spec.Ports {
			addresses = append(addresses, provision.AppInternalAddress{
				Domain:   fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, ns),
				Protocol: string(port.Protocol),
				Port:     port.Port,
				Version:  service.ObjectMeta.Labels[tsuruLabelAppVersion],
				Process:  service.ObjectMeta.Labels[tsuruLabelAppProcess],
			})
		}
	}
	return addresses, nil
}

type nodeFilter struct {
	addresses []string
	metadata  map[string]string
}

func (p *kubernetesProvisioner) listNodesForCluster(cluster *ClusterClient, filter nodeFilter) ([]provision.Node, error) {
	var addressSet set.Set
	if len(filter.addresses) > 0 {
		addressSet = set.FromSlice(filter.addresses)
	}
	controller, err := getClusterController(p, cluster)
	if err != nil {
		return nil, err
	}
	nodeInformer, err := controller.getNodeInformer()
	if err != nil {
		return nil, err
	}
	nodeList, err := nodeInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var nodes []provision.Node
	for i := range nodeList {
		n := &kubernetesNodeWrapper{
			node:    nodeList[i].DeepCopy(),
			prov:    p,
			cluster: cluster,
		}
		matchesAddresses := len(addressSet) == 0 || addressSet.Includes(n.Address())
		matchesMetadata := len(filter.metadata) == 0 || node.HasAllMetadata(n.MetadataNoPrefix(), filter.metadata)
		if matchesAddresses && matchesMetadata {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

func (p *kubernetesProvisioner) ListNodesByFilter(ctx context.Context, filter *provTypes.NodeFilter) ([]provision.Node, error) {
	var nodes []provision.Node
	err := forEachCluster(ctx, func(c *ClusterClient) error {
		clusterNodes, err := p.listNodesForCluster(c, nodeFilter{metadata: filter.Metadata})
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

func (p *kubernetesProvisioner) GetNode(ctx context.Context, address string) (provision.Node, error) {
	_, node, err := p.findNodeByAddress(ctx, address)
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

func (p *kubernetesProvisioner) AddNode(ctx context.Context, opts provision.AddNodeOptions) (err error) {
	client, err := clusterForPool(ctx, opts.Pool)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			servicecommon.RebuildRoutesPoolApps(opts.Pool)
		}
	}()
	hostAddr := tsuruNet.URLToHost(opts.Address)
	conf := getKubeConfig()
	if conf.RegisterNode {
		node := &apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: hostAddr,
			},
		}
		setNodeMetadata(node, opts.Pool, opts.IaaSID, opts.Metadata)
		_, err = client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		if err == nil {
			return nil
		}
		if !k8sErrors.IsAlreadyExists(err) {
			return errors.WithStack(err)
		}
	}
	return p.internalNodeUpdate(ctx, provision.UpdateNodeOptions{
		Address:  hostAddr,
		Metadata: opts.Metadata,
		Pool:     opts.Pool,
	}, opts.IaaSID)
}

func (p *kubernetesProvisioner) RemoveNode(ctx context.Context, opts provision.RemoveNodeOptions) error {
	client, nodeWrapper, err := p.findNodeByAddress(ctx, opts.Address)
	if err != nil {
		return err
	}
	node := nodeWrapper.node
	if opts.Rebalance {
		node.Spec.Unschedulable = true
		_, err = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		var pods []apiv1.Pod
		pods, err = podsFromNode(ctx, client, node.Name, tsuruLabelPrefix+provision.LabelAppPool)
		if err != nil {
			return err
		}
		for _, pod := range pods {
			err = client.CoreV1().Pods(pod.Namespace).Evict(ctx, &policy.Eviction{
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
	err = client.CoreV1().Nodes().Delete(ctx, node.Name, metav1.DeleteOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	servicecommon.RebuildRoutesPoolApps(nodeWrapper.Pool())
	return nil
}

func (p *kubernetesProvisioner) NodeForNodeData(ctx context.Context, nodeData provision.NodeStatusData) (provision.Node, error) {
	return node.FindNodeByAddrs(ctx, p, nodeData.Addrs)
}

func (p *kubernetesProvisioner) findNodeByAddress(ctx context.Context, address string) (*ClusterClient, *kubernetesNodeWrapper, error) {
	var (
		foundNode    *kubernetesNodeWrapper
		foundCluster *ClusterClient
	)
	err := forEachCluster(ctx, func(c *ClusterClient) error {
		if foundNode != nil {
			return nil
		}
		node, err := p.getNodeByAddr(ctx, c, address)
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

func (p *kubernetesProvisioner) UpdateNode(ctx context.Context, opts provision.UpdateNodeOptions) error {
	return p.internalNodeUpdate(ctx, opts, "")
}

func (p *kubernetesProvisioner) internalNodeUpdate(ctx context.Context, opts provision.UpdateNodeOptions, iaasID string) error {
	client, nodeWrapper, err := p.findNodeByAddress(ctx, opts.Address)
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
	_, err = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	return errors.WithStack(err)
}

func (p *kubernetesProvisioner) Deploy(ctx context.Context, args provision.DeployArgs) (string, error) {
	client, err := clusterForPool(ctx, args.App.GetPool())
	if err != nil {
		return "", err
	}
	if err = ensureAppCustomResourceSynced(ctx, client, args.App); err != nil {
		return "", err
	}
	if args.Version.VersionInfo().DeployImage == "" {
		deployPodName := deployPodNameForApp(args.App, args.Version)
		ns, nsErr := client.AppNamespace(ctx, args.App)
		if nsErr != nil {
			return "", nsErr
		}
		defer cleanupPod(tsuruNet.WithoutCancel(ctx), client, deployPodName, ns)
		baseImage, biErr := args.Version.BaseImageName()
		if biErr != nil {
			return "", biErr
		}
		params := createPodParams{
			app:               args.App,
			client:            client,
			podName:           deployPodName,
			sourceImage:       args.Version.VersionInfo().BuildImage,
			destinationImages: []string{baseImage},
			attachOutput:      args.Event,
			attachInput:       strings.NewReader("."),
			inputFile:         "/dev/null",
		}
		err = createDeployPod(ctx, params)
		if err != nil {
			return "", err
		}
		err = args.Version.CommitBaseImage()
		if err != nil {
			return "", err
		}
	}
	manager := &serviceManager{
		client: client,
		writer: args.Event,
	}
	var oldVersionNumber int
	if !args.PreserveVersions {
		oldVersionNumber, err = baseVersionForApp(ctx, client, args.App)
		if err != nil {
			return "", err
		}
	}
	err = servicecommon.RunServicePipeline(ctx, manager, oldVersionNumber, args, nil)
	if err != nil {
		return "", errors.WithStack(err)
	}
	err = ensureAppCustomResourceSynced(ctx, client, args.App)
	if err != nil {
		return "", err
	}
	return args.Version.VersionInfo().DeployImage, nil
}

func (p *kubernetesProvisioner) UpgradeNodeContainer(ctx context.Context, name string, pool string, writer io.Writer) error {
	m := nodeContainerManager{}
	return servicecommon.UpgradeNodeContainer(&m, name, pool, writer)
}

func (p *kubernetesProvisioner) RemoveNodeContainer(ctx context.Context, name string, pool string, writer io.Writer) error {
	err := forEachCluster(ctx, func(cluster *ClusterClient) error {
		return cleanupDaemonSet(ctx, cluster, name, pool)
	})
	if err == provTypes.ErrNoCluster {
		return nil
	}
	return err
}

func (p *kubernetesProvisioner) ExecuteCommand(ctx context.Context, opts provision.ExecOptions) error {
	client, err := clusterForPool(ctx, opts.App.GetPool())
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

	isIsolated := len(opts.Units) == 0
	if isIsolated {
		return runIsolatedCmdPod(ctx, client, eOpts)
	}
	for _, u := range opts.Units {
		eOpts.unit = u
		err := execCommand(ctx, eOpts)
		if err != nil {
			return err
		}
	}
	return nil
}

func runIsolatedCmdPod(ctx context.Context, client *ClusterClient, opts execOpts) error {
	baseName := execCommandPodNameForApp(opts.app)
	labels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
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
	var version appTypes.AppVersion
	if opts.image == "" {
		version, err = servicemanager.AppVersion.LatestSuccessfulVersion(ctx, opts.app)
		if err != nil {
			return errors.WithStack(err)
		}
		opts.image = version.VersionInfo().DeployImage
	}
	appEnvs := provision.EnvsForApp(opts.app, "", false, version)
	var envs []apiv1.EnvVar
	for _, envData := range appEnvs {
		envs = append(envs, apiv1.EnvVar{Name: envData.Name, Value: envData.Value})
	}

	requirements, err := appResourceRequirements(opts.app, client, requirementsFactors{
		overCommit: 1,
	})
	if err != nil {
		return err
	}

	return runPod(ctx, runSinglePodArgs{
		client:       client,
		eventsOutput: opts.eventsOutput,
		stdout:       opts.stdout,
		stderr:       opts.stderr,
		stdin:        opts.stdin,
		termSize:     opts.termSize,
		image:        opts.image,
		labels:       labels,
		cmds:         opts.cmds,
		envs:         envs,
		name:         baseName,
		requirements: requirements,
		app:          opts.app,
	})
}

func (p *kubernetesProvisioner) StartupMessage() (string, error) {
	clusters, err := allClusters(context.TODO())
	if err != nil {
		if err == provTypes.ErrNoCluster {
			return "", nil
		}
		return "", err
	}
	var out string
	for _, c := range clusters {
		nodeList, err := p.listNodesForCluster(c, nodeFilter{})
		if err != nil {
			return "", err
		}
		out += fmt.Sprintf("Kubernetes provisioner on cluster %q - %s:\n", c.Name, c.restConfig.Host)
		if len(nodeList) == 0 {
			out += "    No Kubernetes nodes available\n"
		}
		sort.Slice(nodeList, func(i, j int) bool {
			return nodeList[i].Address() < nodeList[j].Address()
		})
		for _, node := range nodeList {
			out += fmt.Sprintf("    Kubernetes node: %s\n", node.Address())
		}
	}
	return out, nil
}

func (p *kubernetesProvisioner) DeleteVolume(ctx context.Context, volumeName, pool string) error {
	client, err := clusterForPool(ctx, pool)
	if err != nil {
		return err
	}
	return deleteVolume(ctx, client, volumeName)
}

func (p *kubernetesProvisioner) IsVolumeProvisioned(ctx context.Context, volumeName, pool string) (bool, error) {
	client, err := clusterForPool(ctx, pool)
	if err != nil {
		return false, err
	}
	return volumeExists(ctx, client, volumeName)
}

func (p *kubernetesProvisioner) ValidateVolume(ctx context.Context, vol *volumeTypes.Volume) error {
	_, err := validateVolume(vol)
	return err
}

func (p *kubernetesProvisioner) UpdateApp(ctx context.Context, old, new provision.App, w io.Writer) error {
	if old.GetPool() == new.GetPool() {
		return nil
	}

	oldClient, err := clusterForPool(ctx, old.GetPool())
	if errors.Cause(err) == provTypes.ErrNoCluster {
		return nil
	} else if err != nil {
		return err
	}
	newClient, err := clusterForPool(ctx, new.GetPool())
	if err != nil {
		return err
	}
	sameCluster := oldClient.GetCluster().Name == newClient.GetCluster().Name
	sameNamespace := oldClient.PoolNamespace(old.GetPool()) == oldClient.PoolNamespace(new.GetPool())
	if sameCluster && !sameNamespace {
		var volumes []volumeTypes.Volume
		volumes, err = servicemanager.Volume.ListByApp(ctx, old.GetName())
		if err != nil {
			return err
		}
		if len(volumes) > 0 {
			return fmt.Errorf("can't change the pool of an app with binded volumes")
		}
	}
	versions, err := versionsForAppProcess(ctx, oldClient, old, "", false)
	if err != nil {
		return err
	}

	params := updatePipelineParams{
		old:      old,
		new:      new,
		w:        w,
		p:        p,
		versions: versions,
	}
	if !sameCluster {
		if len(versions) > 1 {
			return &tsuruErrors.ValidationError{Message: "can't provision new app with multiple versions, please unify them and try again"}
		}
		actions := []*action.Action{
			&provisionNewApp,
			&restartApp,
			&rebuildAppRoutes,
			&destroyOldApp,
		}

		return action.NewPipeline(actions...).Execute(ctx, params)
	}
	// same cluster and it is not configured with per-pool-namespace, nothing to do.
	if sameNamespace {
		return nil
	}
	if len(versions) > 1 {
		return &tsuruErrors.ValidationError{Message: "can't provision new app with multiple versions, please unify them and try again"}
	}
	actions := []*action.Action{
		&updateAppCR,
		&restartApp,
		&rebuildAppRoutes,
		&removeOldAppResources,
	}

	return action.NewPipeline(actions...).Execute(ctx, params)
}

func (p *kubernetesProvisioner) Shutdown(ctx context.Context) error {
	err := forEachCluster(ctx, func(client *ClusterClient) error {
		stopClusterController(ctx, p, client)
		return nil
	})
	if err == provTypes.ErrNoCluster {
		return nil
	}
	return err
}

func ensureAppCustomResourceSynced(ctx context.Context, client *ClusterClient, a provision.App) error {
	_, err := loadAndEnsureAppCustomResourceSynced(ctx, client, a)
	return err
}

func loadAndEnsureAppCustomResourceSynced(ctx context.Context, client *ClusterClient, a provision.App) (*tsuruv1.App, error) {
	err := ensureNamespace(ctx, client, client.Namespace())
	if err != nil {
		return nil, err
	}
	err = ensureAppCustomResource(ctx, client, a)
	if err != nil {
		return nil, err
	}

	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return nil, err
	}
	appCRD, err := tclient.TsuruV1().Apps(client.Namespace()).Get(ctx, a.GetName(), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	appCRD.Spec.ServiceAccountName = serviceAccountNameForApp(a)

	deploys, err := allDeploymentsForApp(ctx, client, a)
	if err != nil {
		return nil, err
	}
	sort.Slice(deploys, func(i, j int) bool {
		return deploys[i].Name < deploys[j].Name
	})

	svcs, err := allServicesForApp(ctx, client, a)
	if err != nil {
		return nil, err
	}
	sort.Slice(svcs, func(i, j int) bool {
		return svcs[i].Name < svcs[j].Name
	})

	deployments := make(map[string][]string)
	services := make(map[string][]string)
	for _, dep := range deploys {
		l := labelSetFromMeta(&dep.ObjectMeta)
		proc := l.AppProcess()
		deployments[proc] = append(deployments[proc], dep.Name)
	}

	for _, svc := range svcs {
		l := labelSetFromMeta(&svc.ObjectMeta)
		proc := l.AppProcess()
		services[proc] = append(services[proc], svc.Name)
	}

	appCRD.Spec.Services = services
	appCRD.Spec.Deployments = deployments

	pdbs, err := allPDBsForApp(ctx, client, a)
	if err != nil {
		return nil, err
	}
	sort.Slice(pdbs, func(i, j int) bool {
		return pdbs[i].Name < pdbs[j].Name
	})
	appCRD.Spec.PodDisruptionBudgets = make(map[string][]string)
	for _, pdb := range pdbs {
		process := labelSetFromMeta(&pdb.ObjectMeta).AppProcess()
		appCRD.Spec.PodDisruptionBudgets[process] = append(appCRD.Spec.PodDisruptionBudgets[process], pdb.Name)
	}

	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(ctx, a)
	if err != nil && err != appTypes.ErrNoVersionsAvailable {
		return nil, err
	}

	if version != nil {
		appCRD.Spec.Configs, err = normalizeConfigs(version)
		if err != nil {
			return nil, err
		}
	}

	return tclient.TsuruV1().Apps(client.Namespace()).Update(ctx, appCRD, metav1.UpdateOptions{})
}

func ensureAppCustomResource(ctx context.Context, client *ClusterClient, a provision.App) error {
	err := ensureCustomResourceDefinitions(ctx, client)
	if err != nil {
		return err
	}
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	_, err = tclient.TsuruV1().Apps(client.Namespace()).Get(ctx, a.GetName(), metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !k8sErrors.IsNotFound(err) {
		return err
	}
	_, err = tclient.TsuruV1().Apps(client.Namespace()).Create(ctx, &tsuruv1.App{
		ObjectMeta: metav1.ObjectMeta{Name: a.GetName()},
		Spec:       tsuruv1.AppSpec{NamespaceName: client.PoolNamespace(a.GetPool())},
	}, metav1.CreateOptions{})
	return err
}

func ensureCustomResourceDefinitions(ctx context.Context, client *ClusterClient) error {
	extClient, err := ExtensionsClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	toCreate := appCustomResourceDefinition()
	_, err = extClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, toCreate, metav1.CreateOptions{})
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		if k8sErrors.IsNotFound(err) {
			return ensureCustomResourceDefinitionsV1Beta(ctx, client)
		}
		return err
	}
	timeout := time.After(time.Minute)
loop:
	for {
		crd, errGet := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, toCreate.GetName(), metav1.GetOptions{})
		if errGet != nil {
			return errGet
		}
		for _, c := range crd.Status.Conditions {
			if c.Type == extensionsv1.Established && c.Status == extensionsv1.ConditionTrue {
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

func ensureCustomResourceDefinitionsV1Beta(ctx context.Context, client *ClusterClient) error {
	extClient, err := ExtensionsClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	toCreate := appCustomResourceDefinitionV1Beta()
	_, err = extClient.ApiextensionsV1beta1().CustomResourceDefinitions().Create(ctx, toCreate, metav1.CreateOptions{})
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return err
	}
	timeout := time.After(time.Minute)
loop:
	for {
		crd, errGet := extClient.ApiextensionsV1beta1().CustomResourceDefinitions().Get(ctx, toCreate.GetName(), metav1.GetOptions{})
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

func appCustomResourceDefinition() *extensionsv1.CustomResourceDefinition {
	preserveUnknownFields := true
	return &extensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "apps.tsuru.io"},
		Spec: extensionsv1.CustomResourceDefinitionSpec{
			Group: "tsuru.io",
			Scope: extensionsv1.NamespaceScoped,
			Versions: []extensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &extensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &extensionsv1.JSONSchemaProps{
							Type:                   "object",
							XPreserveUnknownFields: &preserveUnknownFields,
						},
					},
				},
			},
			Names: extensionsv1.CustomResourceDefinitionNames{
				Plural:   "apps",
				Singular: "app",
				Kind:     "App",
				ListKind: "AppList",
			},
		},
	}
}

func appCustomResourceDefinitionV1Beta() *v1beta1.CustomResourceDefinition {
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

func normalizeConfigs(version appTypes.AppVersion) (*provTypes.TsuruYamlKubernetesConfig, error) {
	yamlData, err := version.TsuruYamlData()
	if err != nil {
		return nil, err
	}

	config := yamlData.Kubernetes
	if config == nil {
		return nil, nil
	}

	for _, group := range yamlData.Kubernetes.Groups {
		for procName, proc := range group {
			ports, err := getProcessPortsForVersion(version, procName)
			if err == nil {
				proc.Ports = ports
				group[procName] = proc
			}
		}
	}
	return config, nil
}

func EnvsForApp(a provision.App, process string, version appTypes.AppVersion, isDeploy bool) []bind.EnvVar {
	envs := provision.EnvsForApp(a, process, isDeploy, version)
	if isDeploy {
		return envs
	}

	portsConfig, err := getProcessPortsForVersion(version, process)
	if err != nil {
		return envs
	}
	if len(portsConfig) == 0 {
		return removeDefaultPortEnvs(envs)
	}

	portValue := make([]string, len(portsConfig))
	for i, portConfig := range portsConfig {
		targetPort := portConfig.TargetPort
		if targetPort == 0 {
			targetPort = portConfig.Port
		}
		portValue[i] = fmt.Sprintf("%d", targetPort)
	}
	portEnv := bind.EnvVar{Name: fmt.Sprintf("PORT_%s", process), Value: strings.Join(portValue, ",")}
	if !isDefaultPort(portsConfig) {
		envs = removeDefaultPortEnvs(envs)
	}
	return append(envs, portEnv)
}

func removeDefaultPortEnvs(envs []bind.EnvVar) []bind.EnvVar {
	envsWithoutPort := []bind.EnvVar{}
	defaultPortEnvs := provision.DefaultWebPortEnvs()
	for _, env := range envs {
		isDefaultPortEnv := false
		for _, defaultEnv := range defaultPortEnvs {
			if env.Name == defaultEnv.Name {
				isDefaultPortEnv = true
				break
			}
		}
		if !isDefaultPortEnv {
			envsWithoutPort = append(envsWithoutPort, env)
		}
	}

	return envsWithoutPort
}

func isDefaultPort(portsConfig []provTypes.TsuruYamlKubernetesProcessPortConfig) bool {
	if len(portsConfig) != 1 {
		return false
	}

	defaultPort := defaultKubernetesPodPortConfig()
	return portsConfig[0].Protocol == defaultPort.Protocol &&
		portsConfig[0].Port == defaultPort.Port &&
		portsConfig[0].TargetPort == defaultPort.TargetPort
}

func (p *kubernetesProvisioner) HandlesHC() bool {
	return true
}

func (p *kubernetesProvisioner) ToggleRoutable(ctx context.Context, a provision.App, version appTypes.AppVersion, isRoutable bool) error {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return err
	}
	depsData, err := deploymentsDataForApp(ctx, client, a)
	if err != nil {
		return err
	}
	depsForVersion, ok := depsData.versioned[version.Version()]
	if !ok {
		return errors.Errorf("no deployment found for version %v", version.Version())
	}
	for _, depData := range depsForVersion {
		err = toggleRoutableDeployment(ctx, client, version.Version(), depData.dep, isRoutable)
		if err != nil {
			return err
		}
	}
	return ensureAutoScale(ctx, client, a, "")
}

func toggleRoutableDeployment(ctx context.Context, client *ClusterClient, version int, dep *appsv1.Deployment, isRoutable bool) (err error) {
	ls := labelOnlySetFromMetaPrefix(&dep.ObjectMeta, false)
	ls.ToggleIsRoutable(isRoutable)
	ls.SetVersion(version)
	dep.Spec.Paused = true
	dep.ObjectMeta.Labels = ls.WithoutVersion().ToLabels()
	dep.Spec.Template.ObjectMeta.Labels = ls.ToLabels()
	_, err = client.AppsV1().Deployments(dep.Namespace).Update(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if err != nil {
			return
		}
		dep, err = client.AppsV1().Deployments(dep.Namespace).Get(ctx, dep.Name, metav1.GetOptions{})
		if err != nil {
			err = errors.WithStack(err)
			return
		}
		dep.Spec.Paused = false
		_, err = client.AppsV1().Deployments(dep.Namespace).Update(ctx, dep, metav1.UpdateOptions{})
		if err != nil {
			err = errors.WithStack(err)
		}
	}()

	rs, err := activeReplicaSetForDeployment(ctx, client, dep)
	if err != nil {
		if k8sErrors.IsNotFound(errors.Cause(err)) {
			return nil
		}
		return err
	}
	ls = labelOnlySetFromMetaPrefix(&rs.ObjectMeta, false)
	ls.ToggleIsRoutable(isRoutable)
	ls.SetVersion(version)
	rs.ObjectMeta.Labels = ls.ToLabels()
	rs.Spec.Template.ObjectMeta.Labels = ls.ToLabels()
	_, err = client.AppsV1().ReplicaSets(rs.Namespace).Update(ctx, rs, metav1.UpdateOptions{})
	if err != nil {
		return errors.WithStack(err)
	}

	pods, err := podsForReplicaSet(ctx, client, rs)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		ls = labelOnlySetFromMetaPrefix(&pod.ObjectMeta, false)
		ls.ToggleIsRoutable(isRoutable)
		ls.SetVersion(version)
		pod.ObjectMeta.Labels = ls.ToLabels()
		_, err = client.CoreV1().Pods(pod.Namespace).Update(ctx, &pod, metav1.UpdateOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (p *kubernetesProvisioner) DeployedVersions(ctx context.Context, a provision.App) ([]int, error) {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return nil, err
	}
	deps, err := deploymentsDataForApp(ctx, client, a)
	if err != nil {
		return nil, err
	}
	var versions []int
	for v := range deps.versioned {
		versions = append(versions, v)
	}
	return versions, nil
}

func (p *kubernetesProvisioner) RegistryForApp(ctx context.Context, a provision.App) (imgTypes.ImageRegistry, error) {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return "", err
	}
	return client.registry(), nil
}
