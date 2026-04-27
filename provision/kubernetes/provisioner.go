// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"reflect"
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
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/provision/dockercommon"
	_ "github.com/tsuru/tsuru/provision/kubernetes/authplugin/gcpwithproxy" // import custom authplugin that have proxy support
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	"github.com/tsuru/tsuru/streamfmt"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	provTypes "github.com/tsuru/tsuru/types/provision"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
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
	_ provision.MessageProvisioner       = &kubernetesProvisioner{}
	_ provision.VolumeProvisioner        = &kubernetesProvisioner{}
	_ provision.BuilderDeploy            = &kubernetesProvisioner{}
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
	_ provision.JobProvisioner           = &kubernetesProvisioner{}
	_ provision.FileTransferProvisioner  = &kubernetesProvisioner{}

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
	LogLevel   int
	APITimeout time.Duration
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
			disableDefaultNodeSelectorKey: "true",
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
	stopClusterController(p, clusterClient)
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
	stopClusterControllerByName(p, c.Name)
	return nil
}

func (p *kubernetesProvisioner) GetName() string {
	return provisionerName
}

func (p *kubernetesProvisioner) Provision(ctx context.Context, a *appTypes.App) error {
	client, err := clusterForPool(ctx, a.Pool)
	if err != nil {
		return err
	}
	return ensureAppCustomResourceSynced(ctx, client, a)
}

func (p *kubernetesProvisioner) Destroy(ctx context.Context, a *appTypes.App) error {
	client, err := clusterForPool(ctx, a.Pool)
	if err != nil {
		return err
	}
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	app, err := tclient.TsuruV1().Apps(client.Namespace()).Get(ctx, a.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := p.removeResources(ctx, client, app, a); err != nil {
		return err
	}
	return tclient.TsuruV1().Apps(client.Namespace()).Delete(ctx, a.Name, metav1.DeleteOptions{})
}

func (p *kubernetesProvisioner) DestroyVersion(ctx context.Context, a *appTypes.App, version appTypes.AppVersion) error {
	client, err := clusterForPool(ctx, a.Pool)
	if err != nil {
		return err
	}
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	app, err := tclient.TsuruV1().Apps(client.Namespace()).Get(ctx, a.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	return p.removeResourcesFromVersion(ctx, client, app, a, version)
}

func (p *kubernetesProvisioner) removeResourcesFromVersion(ctx context.Context, client *ClusterClient, tsuruApp *tsuruv1.App, app *appTypes.App, version appTypes.AppVersion) error {
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

		secretName := appSecretPrefix + dd.Name
		err = cleanupAppSecret(ctx, client, app, secretName)
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

func (p *kubernetesProvisioner) removeResources(ctx context.Context, client *ClusterClient, tsuruApp *tsuruv1.App, app *appTypes.App) error {
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

		err = cleanupAppSecret(ctx, client, app, appSecretPrefix+dd.Name)
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
	vols, err := servicemanager.Volume.ListByApp(ctx, app.Name)
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
				if b.ID.App != app.Name {
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

func versionsForAppProcess(ctx context.Context, client *ClusterClient, a *appTypes.App, process string, ignoreBaseDepIfStopped bool) ([]appTypes.AppVersion, error) {
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

func changeState(ctx context.Context, a *appTypes.App, process string, version appTypes.AppVersion, state servicecommon.ProcessState, w io.Writer) error {
	client, err := clusterForPool(ctx, a.Pool)
	if err != nil {
		return err
	}
	err = ensureAppCustomResourceSynced(ctx, client, a)
	if err != nil {
		return err
	}

	var versions []appTypes.AppVersion
	if version == nil {
		versions, err = versionsForAppProcess(ctx, client, a, process, false)
		if err != nil {
			return err
		}
		if state.Restart {
			versionsMap := make(map[int]appTypes.AppVersion)
			for _, v := range versions {
				versionsMap[v.VersionInfo().Version] = v
			}

			units, err := GetProvisioner().Units(ctx, a)
			if err != nil {
				return err
			}

			versions = []appTypes.AppVersion{}
			for _, u := range units {
				if val, ok := versionsMap[u.Version]; ok {
					versions = append(versions, val)
					// prevents from adding duplicated versions
					delete(versionsMap, u.Version)
				}
			}
		}
	} else {
		versions = append(versions, version)
	}

	if len(versions) == 0 {
		versionsSlice, err := app.DeployedVersions(ctx, a)
		if err != nil {
			return err
		}
		for _, v := range versionsSlice {
			appVersion, err := servicemanager.AppVersion.VersionByImageOrVersion(ctx, a, strconv.Itoa(v))
			if err != nil {
				return err
			}

			versions = append(versions, appVersion)
		}
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

func patchDeployment(ctx context.Context, client *ClusterClient, a *appTypes.App, patchType types.PatchType, patch []byte, dep *appsv1.Deployment, version appTypes.AppVersion, w io.Writer, process string) error {
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

func changeUnits(ctx context.Context, a *appTypes.App, units int, processName string, version appTypes.AppVersion, w io.Writer) error {
	client, err := clusterForPool(ctx, a.Pool)
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
		w = io.Discard
	}
	newReplicas := int(*dep.Spec.Replicas) + units
	if newReplicas <= 0 {
		streamfmt.FprintlnSectionf(w, "Calling app stop internally as the number of units is zero")
		return GetProvisioner().Stop(ctx, a, processName, version, w)
	}
	patchType, patch, err := replicasPatch(newReplicas)
	if err != nil {
		return err
	}
	streamfmt.FprintlnSectionf(w, "Patching from %d to %d units", *dep.Spec.Replicas, newReplicas)
	return patchDeployment(ctx, client, a, patchType, patch, dep, version, w, processName)
}

func replicasPatch(replicas int) (types.PatchType, []byte, error) {
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

func (p *kubernetesProvisioner) AddUnits(ctx context.Context, a *appTypes.App, units uint, processName string, version appTypes.AppVersion, w io.Writer) error {
	return changeUnits(ctx, a, int(units), processName, version, w)
}

func (p *kubernetesProvisioner) RemoveUnits(ctx context.Context, a *appTypes.App, units uint, processName string, version appTypes.AppVersion, w io.Writer) error {
	return changeUnits(ctx, a, -int(units), processName, version, w)
}

func (p *kubernetesProvisioner) Restart(ctx context.Context, a *appTypes.App, process string, version appTypes.AppVersion, w io.Writer) error {
	return changeState(ctx, a, process, version, servicecommon.ProcessState{Start: true, Restart: true}, w)
}

func (p *kubernetesProvisioner) Start(ctx context.Context, a *appTypes.App, process string, version appTypes.AppVersion, w io.Writer) error {
	return changeState(ctx, a, process, version, servicecommon.ProcessState{Start: true}, w)
}

func (p *kubernetesProvisioner) Stop(ctx context.Context, a *appTypes.App, process string, version appTypes.AppVersion, w io.Writer) error {
	return changeState(ctx, a, process, version, servicecommon.ProcessState{Stop: true}, w)
}

var stateMap = map[apiv1.PodPhase]provTypes.UnitStatus{
	apiv1.PodPending:   provTypes.UnitStatusCreated,
	apiv1.PodRunning:   provTypes.UnitStatusStarted,
	apiv1.PodSucceeded: provTypes.UnitStatusStopped,
	apiv1.PodFailed:    provTypes.UnitStatusError,
	apiv1.PodUnknown:   provTypes.UnitStatusError,
}

func (p *kubernetesProvisioner) podsToUnitsMultiple(pods []apiv1.Pod, baseApps []*appTypes.App) ([]provTypes.Unit, error) {
	if len(pods) == 0 {
		return nil, nil
	}
	appMap := map[string]*appTypes.App{}
	for _, baseApp := range baseApps {
		appMap[baseApp.Name] = baseApp
	}

	var units []provTypes.Unit
	for _, pod := range pods {
		if isTerminating(pod) || podIsAllowedToFail(pod) {
			continue
		}
		l := labelSetFromMeta(&pod.ObjectMeta)

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

		var status provTypes.UnitStatus
		var reason string
		if pod.Status.Phase == apiv1.PodRunning {
			status, reason = extractStatusAndReasonFromContainerStatuses(pod.Status.ContainerStatuses)
		} else {
			status = stateMap[pod.Status.Phase]
			reason = pod.Status.Reason
		}

		createdAt := pod.CreationTimestamp.Time.In(time.UTC)
		units = append(units, provTypes.Unit{
			ID:           pod.Name,
			Name:         pod.Name,
			AppName:      l.AppName(),
			ProcessName:  appProcess,
			Type:         l.AppPlatform(),
			IP:           pod.Status.HostIP,
			InternalIP:   pod.Status.PodIP,
			Status:       status,
			StatusReason: reason,
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

func extractStatusAndReasonFromContainerStatuses(statuses []apiv1.ContainerStatus) (provTypes.UnitStatus, string) {
	for _, containerStatus := range statuses {
		if containerStatus.Ready {
			continue
		}

		return extractStatusAndReasonFromContainerStatus(containerStatus.State, containerStatus.LastTerminationState)

	}
	return provTypes.UnitStatusStarted, ""
}

func extractStatusAndReasonFromContainerStatus(state apiv1.ContainerState, lastTerminationState apiv1.ContainerState) (provTypes.UnitStatus, string) {
	if state.Waiting != nil {
		if state.Waiting.Reason == "CrashLoopBackOff" && lastTerminationState.Terminated != nil {
			if lastTerminationState.Terminated.Reason == "Error" && lastTerminationState.Terminated.ExitCode > 0 {
				return provTypes.UnitStatusError, fmt.Sprintf("exitCode: %d", lastTerminationState.Terminated.ExitCode)
			}
			return provTypes.UnitStatusError, lastTerminationState.Terminated.Reason
		}

		return provTypes.UnitStatusError, state.Waiting.Reason
	}

	if lastTerminationState.Terminated != nil {
		return provTypes.UnitStatusError, lastTerminationState.Terminated.Reason
	}

	return provTypes.UnitStatusStarting, ""
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

func (p *kubernetesProvisioner) Units(ctx context.Context, apps ...*appTypes.App) ([]provTypes.Unit, error) {
	cApps, err := clustersForApps(ctx, apps)
	if err != nil {
		return nil, err
	}
	var units []provTypes.Unit
	for _, cApp := range cApps {
		pods, err := p.podsForApps(ctx, cApp.client, cApp.apps)
		if err != nil {
			return nil, err
		}
		clusterUnits, err := p.podsToUnitsMultiple(pods, cApp.apps)
		if err != nil {
			return nil, err
		}
		units = append(units, clusterUnits...)
	}
	return units, nil
}

func (p *kubernetesProvisioner) podsForApps(ctx context.Context, client *ClusterClient, apps []*appTypes.App) ([]apiv1.Pod, error) {
	inSelectorMap := map[string][]string{}
	for _, a := range apps {
		l, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
			App: a,
			ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
				Prefix: tsuruLabelPrefix,
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

func (p *kubernetesProvisioner) RoutableAddresses(ctx context.Context, a *appTypes.App) ([]appTypes.RoutableAddresses, error) {
	client, err := clusterForPool(ctx, a.Pool)
	if err != nil {
		return nil, err
	}

	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}

	list, err := client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("tsuru.io/app-name=%s", a.Name),
	})
	if err != nil {
		return nil, err
	}

	svcs := make([]apiv1.Service, 0, len(list.Items))

	for _, svc := range list.Items {
		if svc.ObjectMeta.DeletionTimestamp != nil {
			continue
		}

		svcs = append(svcs, svc)
	}

	svcs = filterTsuruControlledServices(svcs)

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

func (p *kubernetesProvisioner) routableAddrForProcess(ctx context.Context, client *ClusterClient, a *appTypes.App, processName, prefix string, version int, svc apiv1.Service) (appTypes.RoutableAddresses, error) {
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

func (p *kubernetesProvisioner) addressesForApp(ctx context.Context, client *ClusterClient, a *appTypes.App, processName string, pubPort int32, version int) ([]*url.URL, error) {
	pods, err := p.podsForApps(ctx, client, []*appTypes.App{a})
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

func (p *kubernetesProvisioner) InternalAddresses(ctx context.Context, a *appTypes.App) ([]appTypes.AppInternalAddress, error) {
	client, err := clusterForPool(ctx, a.Pool)
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

	addresses := []appTypes.AppInternalAddress{}
	for _, service := range svcs {
		// we can't show headless services
		if service.Spec.ClusterIP == "None" {
			continue
		}
		for _, port := range service.Spec.Ports {
			addresses = append(addresses, appTypes.AppInternalAddress{
				Domain:     fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, ns),
				Protocol:   string(port.Protocol),
				Port:       port.Port,
				TargetPort: port.TargetPort.IntValue(),
				Version:    service.ObjectMeta.Labels[tsuruLabelAppVersion],
				Process:    service.ObjectMeta.Labels[tsuruLabelAppProcess],
			})
		}
	}
	return addresses, nil
}

func (p *kubernetesProvisioner) Deploy(ctx context.Context, args provision.DeployArgs) (string, error) {
	client, err := clusterForPool(ctx, args.App.Pool)
	if err != nil {
		return "", err
	}
	if err = ensureAppCustomResourceSynced(ctx, client, args.App); err != nil {
		return "", err
	}
	if args.Version.VersionInfo().DeployImage == "" {
		return "", errors.New("no build image found")
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

func (p *kubernetesProvisioner) ExecuteCommand(ctx context.Context, opts provision.ExecOptions) error {
	client, err := clusterForPool(ctx, opts.App.Pool)
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
		debug:    opts.Debug,
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
	appEnvs := provision.EnvsForAppAndVersion(opts.app, "", version)
	var envs []apiv1.EnvVar
	for _, envData := range appEnvs {
		envs = append(envs, apiv1.EnvVar{Name: envData.Name, Value: envData.Value})
	}

	plan := opts.app.Plan
	pool := opts.app.Pool
	requirements, err := resourceRequirements(&plan, pool, client, requirementsFactors{
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
		out += fmt.Sprintf("Kubernetes provisioner on cluster %q - %s\n", c.Name, c.restConfig.Host)
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

func (p *kubernetesProvisioner) UpdateApp(ctx context.Context, old, new *appTypes.App, w io.Writer) error {
	if old.Pool == new.Pool {
		return nil
	}

	oldClient, err := clusterForPool(ctx, old.Pool)
	if errors.Cause(err) == provTypes.ErrNoCluster {
		return nil
	} else if err != nil {
		return err
	}
	newClient, err := clusterForPool(ctx, new.Pool)
	if err != nil {
		return err
	}
	sameCluster := oldClient.GetCluster().Name == newClient.GetCluster().Name
	sameNamespace := oldClient.PoolNamespace(old.Pool) == oldClient.PoolNamespace(new.Pool)
	if sameCluster && !sameNamespace {
		var volumes []volumeTypes.Volume
		volumes, err = servicemanager.Volume.ListByApp(ctx, old.Name)
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
		stopClusterController(p, client)
		return nil
	})
	if err == provTypes.ErrNoCluster {
		return nil
	}
	return err
}

func ensureAppCustomResourceSynced(ctx context.Context, client *ClusterClient, a *appTypes.App) error {
	err := ensureNamespace(ctx, client, client.Namespace())
	if err != nil {
		return err
	}
	err = ensureAppCustomResource(ctx, client, a)
	if err != nil {
		return err
	}

	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	appCRD, err := tclient.TsuruV1().Apps(client.Namespace()).Get(ctx, a.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	originalAPPCRD := appCRD.DeepCopy()
	appCRD.Spec.ServiceAccountName = serviceAccountNameForApp(a)

	deploys, err := allDeploymentsForApp(ctx, client, a)
	if err != nil {
		return err
	}
	sort.Slice(deploys, func(i, j int) bool {
		return deploys[i].Name < deploys[j].Name
	})

	svcs, err := allServicesForApp(ctx, client, a)
	if err != nil {
		return err
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
		return err
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
		return err
	}

	if version != nil {
		appCRD.Spec.Configs, err = normalizeConfigs(version)
		if err != nil {
			return err
		}
	}

	if reflect.DeepEqual(originalAPPCRD.Spec, appCRD.Spec) {
		return nil
	}

	_, err = tclient.TsuruV1().Apps(client.Namespace()).Update(ctx, appCRD, metav1.UpdateOptions{})
	return err
}

func ensureAppCustomResource(ctx context.Context, client *ClusterClient, a *appTypes.App) error {
	err := ensureCustomResourceDefinitions(ctx, client)
	if err != nil {
		return err
	}
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	_, err = tclient.TsuruV1().Apps(client.Namespace()).Get(ctx, a.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !k8sErrors.IsNotFound(err) {
		return err
	}
	_, err = tclient.TsuruV1().Apps(client.Namespace()).Create(ctx, &tsuruv1.App{
		ObjectMeta: metav1.ObjectMeta{Name: a.Name},
		Spec:       tsuruv1.AppSpec{NamespaceName: client.PoolNamespace(a.Pool)},
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

func EnvsForApp(a *appTypes.App, process string, version appTypes.AppVersion) []bindTypes.EnvVar {
	envs := provision.EnvsForAppAndVersion(a, process, version)

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
	portEnv := bindTypes.EnvVar{
		Name:   fmt.Sprintf("PORT_%s", process),
		Value:  strings.Join(portValue, ","),
		Public: true,
	}
	if !isDefaultPort(portsConfig) {
		envs = removeDefaultPortEnvs(envs)
	}
	return append(envs, portEnv)
}

func removeDefaultPortEnvs(envs []bindTypes.EnvVar) []bindTypes.EnvVar {
	envsWithoutPort := []bindTypes.EnvVar{}
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

func (p *kubernetesProvisioner) ToggleRoutable(ctx context.Context, a *appTypes.App, version appTypes.AppVersion, isRoutable bool) error {
	client, err := clusterForPool(ctx, a.Pool)
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
		err = toggleRoutableDeployment(ctx, client, depData.dep, isRoutable)
		if err != nil {
			return err
		}
	}
	return ensureAutoScale(ctx, client, a, "")
}

func toggleRoutableDeployment(ctx context.Context, client *ClusterClient, dep *appsv1.Deployment, isRoutable bool) (err error) {
	isRouteableLabel := "tsuru.io/is-routable"
	isRouteableValue := strconv.FormatBool(isRoutable)

	dep.Spec.Paused = true
	dep.ObjectMeta.Labels[isRouteableLabel] = isRouteableValue
	dep.Spec.Template.ObjectMeta.Labels[isRouteableLabel] = isRouteableValue
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

	rs.ObjectMeta.Labels[isRouteableLabel] = isRouteableValue
	rs.Spec.Template.ObjectMeta.Labels[isRouteableLabel] = isRouteableValue
	_, err = client.AppsV1().ReplicaSets(rs.Namespace).Update(ctx, rs, metav1.UpdateOptions{})
	if err != nil {
		return errors.WithStack(err)
	}

	pods, err := podsForReplicaSet(ctx, client, rs)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		pod.ObjectMeta.Labels[isRouteableLabel] = isRouteableValue
		_, err = client.CoreV1().Pods(pod.Namespace).Update(ctx, &pod, metav1.UpdateOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (p *kubernetesProvisioner) DeployedVersions(ctx context.Context, a *appTypes.App) ([]int, error) {
	client, err := clusterForPool(ctx, a.Pool)
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

func (p *kubernetesProvisioner) RegistryForPool(ctx context.Context, pool string) (imgTypes.ImageRegistry, error) {
	client, err := clusterForPool(ctx, pool)
	if err != nil {
		return "", err
	}
	return client.Registry(), nil
}

func (p *kubernetesProvisioner) UploadFile(ctx context.Context, app *appTypes.App, unit string, file []byte, filepath string) error {
	client, err := clusterForPool(ctx, app.Pool)
	if err != nil {
		return err
	}
	namespace, err := client.AppNamespace(ctx, app)
	if err != nil {
		return err
	}

	pathDir := path.Dir(filepath)
	untarCmd := []string{"tar", "xvf", "-", "-C", pathDir}

	request := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(unit).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&apiv1.PodExecOptions{
			Command:   untarCmd,
			Container: "",
			Stdin:     true,
			Stdout:    false,
			Stderr:    true,
			TTY:       false,
		}, metav1.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(client.restConfig, "POST", request.URL())
	if err != nil {
		return err
	}

	var stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  bytes.NewReader(file),
		Stdout: io.Discard,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil {
		return fmt.Errorf("Failed to upload file: %v (stderr: %s)", err, stderr.String())
	}

	return nil
}
