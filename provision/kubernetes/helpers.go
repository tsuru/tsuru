// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	tsuruLabelPrefix          = "tsuru.io/"
	tsuruInProgressTaint      = tsuruLabelPrefix + "inprogress"
	tsuruNodeDisabledTaint    = tsuruLabelPrefix + "disabled"
	tsuruExtraLabelsMeta      = tsuruLabelPrefix + "extra-labels"
	tsuruExtraAnnotationsMeta = tsuruLabelPrefix + "extra-annotations"
	tsuruLabelAppName         = tsuruLabelPrefix + provision.LabelAppName
	tsuruLabelAppVersion      = tsuruLabelPrefix + provision.LabelAppVersion
	tsuruLabelAppProcess      = tsuruLabelPrefix + provision.LabelAppProcess
	tsuruLabelIsBuild         = tsuruLabelPrefix + provision.LabelIsBuild
	tsuruLabelIsDeploy        = tsuruLabelPrefix + provision.LabelIsDeploy
	replicaDepRevision        = "deployment.kubernetes.io/revision"
	kubeLabelNameMaxLen       = 55
)

var svcIgnoredLabels = []string{
	tsuruLabelPrefix + "router-lb",
	tsuruLabelPrefix + "external-controller",
}

var kubeNameRegex = regexp.MustCompile(`(?i)[^a-z0-9.-]`)

func validKubeName(name string) string {
	return strings.ToLower(kubeNameRegex.ReplaceAllString(name, "-"))
}

func serviceAccountNameForApp(a provision.App) string {
	name := validKubeName(a.GetName())
	return fmt.Sprintf("app-%s", name)
}

func serviceAccountNameForNodeContainer(nodeContainer nodecontainer.NodeContainerConfig) string {
	name := validKubeName(nodeContainer.Name)
	return fmt.Sprintf("node-container-%s", name)
}

func deploymentNameForApp(a provision.App, process string, version int) string {
	return appProcessName(a, process, version, "")
}

func deploymentNameForAppBase(a provision.App, process string) string {
	return appProcessName(a, process, 0, "")
}

func serviceNameForApp(a provision.App, process string, version int) string {
	return appProcessName(a, process, version, "")
}

func serviceNameForAppBase(a provision.App, process string) string {
	return appProcessName(a, process, 0, "")
}

func headlessServiceName(a provision.App, process string) string {
	return appProcessName(a, process, 0, "units")
}

func deployPodNameForApp(a provision.App, version appTypes.AppVersion) string {
	name := validKubeName(a.GetName())
	return fmt.Sprintf("%s-v%d-deploy", name, version.Version())
}

func buildPodNameForApp(a provision.App, version appTypes.AppVersion) string {
	name := validKubeName(a.GetName())
	return fmt.Sprintf("%s-v%d-build", name, version.Version())
}

func hpaNameForApp(a provision.App, process string) string {
	return appProcessName(a, process, 0, "")
}

func vpaNameForApp(a provision.App, process string) string {
	return appProcessName(a, process, 0, "")
}

func appProcessName(a provision.App, process string, version int, suffix string) string {
	name := validKubeName(a.GetName())
	processVersion := validKubeName(process)
	if version > 0 {
		processVersion = fmt.Sprintf("%s-v%d", processVersion, version)
	} else if suffix != "" {
		processVersion = fmt.Sprintf("%s-%s", processVersion, suffix)
	}
	label := fmt.Sprintf("%s-%s", name, processVersion)
	if len(label) > kubeLabelNameMaxLen {
		h := sha256.New()
		h.Write([]byte(processVersion))
		hash := fmt.Sprintf("%x", h.Sum(nil))
		maxLen := kubeLabelNameMaxLen - len(name) - 1
		if len(hash) > maxLen {
			hash = hash[:maxLen]
		}
		label = fmt.Sprintf("%s-%s", name, hash)
	}
	return label
}

func execCommandPodNameForApp(a provision.App) string {
	name := validKubeName(a.GetName())
	return fmt.Sprintf("%s-isolated-run", name)
}

func daemonSetName(name, pool string) string {
	name = validKubeName(name)
	pool = validKubeName(pool)
	if pool == "" {
		return fmt.Sprintf("node-container-%s-all", name)
	}
	return fmt.Sprintf("node-container-%s-pool-%s", name, pool)
}

func volumeName(name string) string {
	return fmt.Sprintf("%s-tsuru", name)
}

func volumeClaimName(name string) string {
	return fmt.Sprintf("%s-tsuru-claim", name)
}

func registrySecretName(registry string) string {
	registry = validKubeName(registry)
	if registry == "" {
		return "registry"
	}

	return fmt.Sprintf("registry-%s", registry)
}

func waitFor(ctx context.Context, fn func() (bool, error), onCancel func() error) error {
	start := time.Now()
	for {
		done, err := fn()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			if onCancel == nil {
				err = errors.Wrapf(ctx.Err(), "canceled after %v", time.Since(start))
			} else {
				err = errors.Wrapf(ctx.Err(), "canceled after %v: %v", time.Since(start), onCancel())
			}
			return err
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func podsForAppProcess(ctx context.Context, client *ClusterClient, ns string, selector map[string]string) (*apiv1.PodList, error) {
	podList, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(selector)).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return podList, nil
}

func allNewPodsRunning(ctx context.Context, client *ClusterClient, a provision.App, process string, dep *appsv1.Deployment, version appTypes.AppVersion) (bool, error) {
	replica, err := activeReplicaSetForDeployment(ctx, client, dep)
	if err != nil {
		if k8sErrors.IsNotFound(errors.Cause(err)) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	pods, err := podsForReplicaSet(ctx, client, replica)
	if err != nil {
		return false, err
	}
	for _, pod := range pods {
		if pod.Status.Phase != apiv1.PodRunning {
			return false, nil
		}
	}
	return len(pods) > 0, nil
}

func activeReplicaSetForDeployment(ctx context.Context, client *ClusterClient, dep *appsv1.Deployment) (*appsv1.ReplicaSet, error) {
	depRevision := dep.Annotations[replicaDepRevision]

	replicaSets, err := getAllReplicasets(ctx, client, dep)
	if err != nil {
		return nil, err
	}
	for _, rs := range replicaSets {
		if rs.Annotations != nil && rs.Annotations[replicaDepRevision] == depRevision {
			return &rs, nil
		}
	}
	return nil, k8sErrors.NewNotFound(appsv1.Resource("replicaset"), fmt.Sprintf("deployment: %v, revision: %v", dep.Name, depRevision))
}

func getAllReplicasets(ctx context.Context, client kubernetes.Interface, dep *appsv1.Deployment) ([]appsv1.ReplicaSet, error) {
	sel, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	replicaSets, err := client.AppsV1().ReplicaSets(dep.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: sel.String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return replicaSets.Items, nil
}

func podsForReplicaSet(ctx context.Context, client *ClusterClient, rs *appsv1.ReplicaSet) ([]apiv1.Pod, error) {
	sel, err := metav1.LabelSelectorAsSelector(rs.Spec.Selector)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	pods, err := client.CoreV1().Pods(rs.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: sel.String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return pods.Items, nil
}

type podErrorMessage struct {
	message string
	pod     apiv1.Pod
}

func notReadyPodEvents(ctx context.Context, client *ClusterClient, ns string, selector map[string]string) ([]podErrorMessage, error) {
	pods, err := podsForAppProcess(ctx, client, ns, selector)
	if err != nil {
		return nil, err
	}
	return notReadyPodEventsForPods(ctx, client, pods.Items)
}

func notReadyPodEventsForPod(ctx context.Context, client *ClusterClient, podName, ns string) ([]podErrorMessage, error) {
	pod, err := client.CoreV1().Pods(ns).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return notReadyPodEventsForPods(ctx, client, []apiv1.Pod{*pod})
}

func notReadyPodEventsForPods(ctx context.Context, client *ClusterClient, pods []apiv1.Pod) ([]podErrorMessage, error) {
	const (
		eventReasonUnhealthy        = "Unhealthy"
		eventReasonFailedScheduling = "FailedScheduling"
	)

	var messages []podErrorMessage
	for _, pod := range pods {
		for _, cond := range pod.Status.Conditions {
			if cond.Type == apiv1.PodReady && cond.Status != apiv1.ConditionTrue {
				msg := fmt.Sprintf("Pod %q not ready", pod.Name)
				if cond.Message != "" {
					msg = fmt.Sprintf("%s: %s", msg, cond.Message)
				} else if cond.Reason != "" {
					msg = fmt.Sprintf("%s: %s", msg, cond.Reason)
				}
				messages = append(messages, podErrorMessage{pod: pod, message: msg})
			}
		}
		for _, contStatus := range pod.Status.ContainerStatuses {
			termState := contStatus.State.Terminated
			if termState == nil && contStatus.State.Waiting != nil && contStatus.LastTerminationState.Terminated != nil {
				termState = contStatus.LastTerminationState.Terminated
			}
			if termState == nil {
				continue
			}
			msg := fmt.Sprintf("Pod %q has crashed %d times: Exited with status %d", pod.Name, contStatus.RestartCount, termState.ExitCode)
			if termState.Message != "" {
				msg = fmt.Sprintf("%s: %s", msg, termState.Message)
			}
			messages = append(messages, podErrorMessage{pod: pod, message: msg})
		}
		lastEvt, err := lastEventForPod(ctx, client, &pod)
		if err != nil {
			return messages, err
		}
		if lastEvt == nil {
			continue
		}
		var msg string
		switch lastEvt.Reason {
		case eventReasonUnhealthy:
			msg = fmt.Sprintf("Pod %q failed health check", pod.Name)
			probeMsg := probeMsg(pod)
			if probeMsg != "" {
				msg = fmt.Sprintf("%s, %s", msg, probeMsg)
			}
		case eventReasonFailedScheduling:
			msg = fmt.Sprintf("Pod %q could not be scheduled", pod.Name)
		default:
			if lastEvt.Type == apiv1.EventTypeWarning {
				msg = fmt.Sprintf("Pod %q with warning", pod.Name)
			}
		}
		if msg != "" {
			if lastEvt.Message != "" {
				msg = fmt.Sprintf("%s: %s", msg, lastEvt.Message)
			}
			messages = append(messages, podErrorMessage{pod: pod, message: msg})
		}
	}
	return messages, nil
}

func probeMsg(pod apiv1.Pod) string {
	if len(pod.Spec.Containers) == 0 {
		return ""
	}
	probe := pod.Spec.Containers[0].ReadinessProbe
	if probe == nil {
		return ""
	}
	if probe.Handler.HTTPGet != nil {
		return fmt.Sprintf("HTTP GET to %s on port %s", probe.Handler.HTTPGet.Path, probe.Handler.HTTPGet.Port.String())
	}
	if probe.Handler.TCPSocket != nil {
		return fmt.Sprintf("TCP connect on port %s", probe.Handler.TCPSocket.Port.String())
	}
	if probe.Handler.Exec != nil {
		return fmt.Sprintf("Command exec %q", probe.Handler.Exec.Command)
	}
	return ""
}

func waitForPodContainersRunning(ctx context.Context, client *ClusterClient, origPod *apiv1.Pod, namespace string) error {
	return waitFor(ctx, func() (bool, error) {
		err := waitForPod(ctx, client, origPod, namespace, true)
		if err != nil {
			return true, errors.WithStack(err)
		}
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, origPod.Name, metav1.GetOptions{})
		if err != nil {
			return true, errors.WithStack(err)
		}
		allRunning := true
		for _, contStatus := range pod.Status.ContainerStatuses {
			if contStatus.State.Terminated != nil {
				termData := contStatus.State.Terminated
				return true, errors.Errorf("unexpected container %q termination: Exit %d - Reason: %q - Message: %q", contStatus.Name, termData.ExitCode, termData.Reason, termData.Message)
			}
			if contStatus.State.Running == nil {
				allRunning = false
			}
		}
		if allRunning {
			return true, nil
		}
		return false, nil
	}, func() error {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, origPod.Name, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		return newInvalidPodPhaseError(ctx, client, pod, namespace)
	})
}

func eventsForPod(ctx context.Context, client *ClusterClient, pod *apiv1.Pod) (*apiv1.EventList, error) {
	eventsInterface := client.CoreV1().Events(pod.Namespace)
	selector := eventsInterface.GetFieldSelector(&pod.Name, &pod.Namespace, nil, nil)
	options := metav1.ListOptions{
		FieldSelector: selector.String(),
	}
	return eventsInterface.List(ctx, options)
}

func lastEventForPod(ctx context.Context, client *ClusterClient, pod *apiv1.Pod) (*apiv1.Event, error) {
	events, err := eventsForPod(ctx, client, pod)
	if err != nil {
		return nil, err
	}

	sort.Slice(events.Items, func(i, j int) bool {
		return events.Items[i].LastTimestamp.Before(&events.Items[j].LastTimestamp)
	})

	if len(events.Items) > 0 {
		return &events.Items[len(events.Items)-1], nil
	}
	return nil, nil
}

func newInvalidPodPhaseError(ctx context.Context, client *ClusterClient, pod *apiv1.Pod, namespace string) error {
	phaseWithMsg := fmt.Sprintf("%q", pod.Status.Phase)
	if pod.Status.Message != "" {
		phaseWithMsg = fmt.Sprintf("%s(%q)", phaseWithMsg, pod.Status.Message)
	}
	retErr := errors.Errorf("invalid pod phase %s", phaseWithMsg)
	lastEvt, err := lastEventForPod(ctx, client, pod)
	if err == nil && lastEvt != nil {
		retErr = errors.Errorf("%v - last event: %s", retErr, lastEvt.Message)
	}
	return retErr
}

func podContainerNames(pod *apiv1.Pod) []string {
	var names []string
	for _, cont := range pod.Spec.Containers {
		names = append(names, cont.Name)
	}
	return names
}

func waitForPod(ctx context.Context, client *ClusterClient, origPod *apiv1.Pod, namespace string, returnOnRunning bool) error {
	validContSet := set.FromSlice(podContainerNames(origPod))
	return waitFor(ctx, func() (bool, error) {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, origPod.Name, metav1.GetOptions{})
		if err != nil {
			return true, errors.WithStack(err)
		}
		if pod.Status.Phase == apiv1.PodPending {
			return false, nil
		}
		switch pod.Status.Phase {
		case apiv1.PodRunning:
			if returnOnRunning {
				return true, nil
			}
			allDone := len(validContSet) > 0
			for _, contStatus := range pod.Status.ContainerStatuses {
				if !validContSet.Includes(contStatus.Name) {
					continue
				}
				termData := contStatus.State.Terminated
				if termData == nil {
					allDone = false
					break
				}
				if termData.ExitCode != 0 {
					invalidErr := newInvalidPodPhaseError(ctx, client, pod, namespace)
					return true, errors.Wrapf(invalidErr, "unexpected container %q termination: Exit %d - Reason: %q - Message: %q", contStatus.Name, termData.ExitCode, termData.Reason, termData.Message)
				}
			}
			return allDone, nil
		case apiv1.PodUnknown:
			fallthrough
		case apiv1.PodFailed:
			return true, newInvalidPodPhaseError(ctx, client, pod, namespace)
		}
		return true, nil
	}, func() error {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, origPod.Name, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		return newInvalidPodPhaseError(ctx, client, pod, namespace)
	})
}

func cleanupPods(ctx context.Context, client *ClusterClient, opts metav1.ListOptions, controller metav1.Object) error {
	pods, err := client.CoreV1().Pods(controller.GetNamespace()).List(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pod := range pods.Items {
		if !metav1.IsControlledBy(&pod, controller) {
			continue
		}

		err = client.CoreV1().Pods(controller.GetNamespace()).Delete(ctx, pod.Name, metav1.DeleteOptions{})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
	}
	return nil
}

func propagationPtr(p metav1.DeletionPropagation) *metav1.DeletionPropagation {
	return &p
}

func cleanupReplicas(ctx context.Context, client *ClusterClient, dep *appsv1.Deployment) error {
	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return err
	}
	listOpts := metav1.ListOptions{
		LabelSelector: selector.String(),
	}

	replicas, err := client.AppsV1().ReplicaSets(dep.Namespace).List(ctx, listOpts)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, replica := range replicas.Items {
		if !metav1.IsControlledBy(&replica, dep) {
			continue
		}

		err = client.AppsV1().ReplicaSets(dep.Namespace).Delete(ctx, replica.Name, metav1.DeleteOptions{
			PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
		})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
	}
	return cleanupPods(ctx, client, listOpts, dep)
}

func baseVersionForApp(ctx context.Context, client *ClusterClient, a provision.App) (int, error) {
	depData, err := deploymentsDataForApp(ctx, client, a)
	if err != nil {
		return 0, err
	}

	if depData.base.dep == nil {
		return 0, nil
	}

	return depData.base.version, nil
}

func allDeploymentsForApp(ctx context.Context, client *ClusterClient, a provision.App) ([]appsv1.Deployment, error) {
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}
	return allDeploymentsForAppNS(ctx, client, ns, a)
}

func allDeploymentsForAppNS(ctx context.Context, client *ClusterClient, ns string, a provision.App) ([]appsv1.Deployment, error) {
	svcLabels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	selector := labels.SelectorFromSet(labels.Set(svcLabels.ToAppSelector()))
	deps, err := client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	return deps.Items, nil
}

func allServicesForApp(ctx context.Context, client *ClusterClient, a provision.App) ([]apiv1.Service, error) {
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}
	return allServicesForAppNS(ctx, client, ns, a)
}

func allServicesForAppNS(ctx context.Context, client *ClusterClient, ns string, a provision.App) ([]apiv1.Service, error) {
	svcLabels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	selector := labels.SelectorFromSet(labels.Set(svcLabels.ToAppSelector()))
	svcs, err := client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return filterTsuruControlledServices(svcs.Items), nil
}

func allServicesForAppInformer(ctx context.Context, informer v1informers.ServiceInformer, ns string, a provision.App) ([]apiv1.Service, error) {
	svcLabels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	selector := labels.SelectorFromSet(labels.Set(svcLabels.ToAppSelector()))
	svcs, err := informer.Lister().Services(ns).List(selector)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var result []apiv1.Service
	for _, svc := range svcs {
		result = append(result, *svc.DeepCopy())
	}
	return filterTsuruControlledServices(result), nil
}

func filterTsuruControlledServices(svcs []apiv1.Service) []apiv1.Service {
	result := make([]apiv1.Service, 0, len(svcs))
svcsLoop:
	for _, svc := range svcs {
		for _, label := range svcIgnoredLabels {
			_, hasLabel := svc.Labels[label]
			if hasLabel {
				continue svcsLoop
			}
		}
		result = append(result, svc)
	}
	return result
}

func allDeploymentsForAppProcess(ctx context.Context, client *ClusterClient, a provision.App, process string) ([]appsv1.Deployment, error) {
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}

	svcLabels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App:     a,
		Process: process,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	selector := labels.SelectorFromSet(labels.Set(svcLabels.ToAllVersionsSelector()))
	deps, err := client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}

	return deps.Items, nil
}

type groupedDeployments struct {
	versioned map[int][]deploymentInfo
	base      deploymentInfo
	count     int
}

type deploymentInfo struct {
	dep        *appsv1.Deployment
	process    string
	version    int
	isLegacy   bool
	isBase     bool
	isRoutable bool
	replicas   int
}

func deploymentsDataForProcess(ctx context.Context, client *ClusterClient, a provision.App, process string) (groupedDeployments, error) {
	deps, err := allDeploymentsForAppProcess(ctx, client, a, process)
	if err != nil {
		return groupedDeployments{}, err
	}
	return groupDeployments(deps), nil
}

func deploymentsDataForApp(ctx context.Context, client *ClusterClient, a provision.App) (groupedDeployments, error) {
	deps, err := allDeploymentsForApp(ctx, client, a)
	if err != nil {
		return groupedDeployments{}, err
	}
	return groupDeployments(deps), nil
}

func groupDeployments(deps []appsv1.Deployment) groupedDeployments {
	result := groupedDeployments{
		versioned: make(map[int][]deploymentInfo),
	}
	result.count = len(deps)
	for i, dep := range deps {
		labels := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
		version := labels.AppVersion()
		isLegacy := false
		isBase := labels.IsBase()
		isRoutable := labels.IsRoutable()
		if version == 0 {
			isBase = true
			isLegacy = true
			isRoutable = true
			if len(dep.Spec.Template.Spec.Containers) == 0 {
				continue
			}
			_, tag := image.SplitImageName(dep.Spec.Template.Spec.Containers[0].Image)
			version, _ = strconv.Atoi(strings.TrimPrefix(tag, "v"))
		}
		if version == 0 {
			continue
		}
		di := deploymentInfo{
			dep:        &deps[i],
			version:    version,
			isLegacy:   isLegacy,
			isBase:     isBase,
			isRoutable: isRoutable,
			process:    labels.AppProcess(),
		}
		if dep.Spec.Replicas != nil {
			di.replicas = int(*dep.Spec.Replicas)
		}
		result.versioned[version] = append(result.versioned[version], di)
		if isBase {
			result.base = di
		}
	}
	return result
}

func deploymentForVersion(ctx context.Context, client *ClusterClient, a provision.App, process string, versionNumber int) (*appsv1.Deployment, error) {
	groupedDeps, err := deploymentsDataForProcess(ctx, client, a, process)
	if err != nil {
		return nil, err
	}

	depsData := groupedDeps.versioned[versionNumber]
	if len(depsData) == 0 {
		return nil, k8sErrors.NewNotFound(appsv1.Resource("deployment"), fmt.Sprintf("app: %v, process: %v, version: %v", a.GetName(), process, versionNumber))
	}

	if len(depsData) > 1 {
		return nil, errors.Errorf("two many deployments for same version %d and process %q: %d", versionNumber, process, len(depsData))
	}

	return depsData[0].dep, nil
}

func cleanupSingleDeployment(ctx context.Context, client *ClusterClient, dep *appsv1.Deployment) error {
	err := client.AppsV1().Deployments(dep.Namespace).Delete(ctx, dep.Name, metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}
		return errors.WithStack(err)
	}

	return cleanupReplicas(ctx, client, dep)
}

func cleanupDeployment(ctx context.Context, client *ClusterClient, a provision.App, process string, versionNumber int) error {
	dep, err := deploymentForVersion(ctx, client, a, process, versionNumber)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	err = client.AppsV1().Deployments(dep.Namespace).Delete(ctx, dep.Name, metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}

	return cleanupReplicas(ctx, client, dep)
}

func allServicesForAppProcess(ctx context.Context, client *ClusterClient, a provision.App, process string) ([]apiv1.Service, error) {
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}

	svcLabels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App:     a,
		Process: process,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	selector := labels.SelectorFromSet(labels.Set(svcLabels.ToAllVersionsSelector()))
	svcs, err := client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}

	return svcs.Items, nil
}

func cleanupServices(ctx context.Context, client *ClusterClient, a provision.App, process string, versionNumber int) error {
	svcs, err := allServicesForAppProcess(ctx, client, a, process)
	if err != nil {
		return err
	}

	deps, err := allDeploymentsForAppProcess(ctx, client, a, process)
	if err != nil {
		return err
	}
	processInUse := len(deps) > 1

	var toDelete []apiv1.Service
	for _, svc := range svcs {
		labels := labelSetFromMeta(&svc.ObjectMeta)
		svcVersion := labels.AppVersion()
		if svcVersion == versionNumber || !processInUse {
			toDelete = append(toDelete, svc)
		}
	}

	for _, svc := range toDelete {
		err = client.CoreV1().Services(svc.Namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{
			PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
		})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
	}
	return nil
}

func cleanupDaemonSet(ctx context.Context, client *ClusterClient, name, pool string) error {
	dsName := daemonSetName(name, pool)
	ns := client.PoolNamespace(pool)
	ds, err := client.AppsV1().DaemonSets(ns).Get(ctx, dsName, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}
		return errors.WithStack(err)
	}
	err = client.AppsV1().DaemonSets(ns).Delete(ctx, dsName, metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	ls := provision.NodeContainerLabels(provision.NodeContainerLabelsOpts{
		Name:        name,
		Pool:        pool,
		Provisioner: provisionerName,
		Prefix:      tsuruLabelPrefix,
	})
	return cleanupPods(ctx, client, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToNodeContainerSelector())).String(),
	}, ds)
}

func cleanupPod(ctx context.Context, client *ClusterClient, podName, namespace string) error {
	noWait := int64(0)
	err := client.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{
		GracePeriodSeconds: &noWait,
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		log.Errorf("[cleanupPod] Deferred cleanup action failed %s", err.Error())
		return errors.WithStack(err)
	}
	return nil
}

func podsFromNode(ctx context.Context, client *ClusterClient, nodeName, labelFilter string) ([]apiv1.Pod, error) {
	restCli, err := rest.RESTClientFor(client.restConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	opts := metav1.ListOptions{
		LabelSelector: labelFilter,
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"spec.nodeName": nodeName,
		}).String(),
	}
	var podList apiv1.PodList
	err = restCli.Get().
		Resource("pods").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&podList)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return podList.Items, nil
}

func appPodsFromNode(ctx context.Context, client *ClusterClient, nodeName string) ([]apiv1.Pod, error) {
	l := provision.LabelSet{Prefix: tsuruLabelPrefix}
	l.SetIsService()
	serviceSelector := fields.SelectorFromSet(fields.Set(l.ToIsServiceSelector())).String()
	labelFilter := fmt.Sprintf("%s,%s%s", serviceSelector, tsuruLabelPrefix, provision.LabelAppPool)
	return podsFromNode(ctx, client, nodeName, labelFilter)
}

func getServicePorts(svcInformer v1informers.ServiceInformer, srvName, namespace string) ([]int32, error) {
	if namespace == "" {
		namespace = "default"
	}
	srv, err := svcInformer.Lister().Services(namespace).Get(srvName)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	svcPorts := make([]int32, len(srv.Spec.Ports))
	for i, p := range srv.Spec.Ports {
		svcPorts[i] = p.NodePort
	}
	return svcPorts, nil
}

func labelOnlySetFromMeta(meta *metav1.ObjectMeta) *provision.LabelSet {
	return labelOnlySetFromMetaPrefix(meta, true)
}

func labelOnlySetFromMetaPrefix(meta *metav1.ObjectMeta, includePrefix bool) *provision.LabelSet {
	labels := make(map[string]string, len(meta.Labels)+len(meta.Annotations))
	rawLabels := make(map[string]string)
	for k, v := range meta.Labels {
		trimmedKey := strings.TrimPrefix(k, tsuruLabelPrefix)
		if trimmedKey != k {
			if !includePrefix {
				k = trimmedKey
			}
			labels[k] = v
		} else {
			rawLabels[k] = v
		}
	}
	return &provision.LabelSet{Labels: labels, RawLabels: rawLabels, Prefix: tsuruLabelPrefix}
}

func labelSetFromMeta(meta *metav1.ObjectMeta) *provision.LabelSet {
	ls := labelOnlySetFromMeta(meta)
	if ls.Labels == nil {
		ls.Labels = make(map[string]string)
	}
	for k, v := range meta.Annotations {
		ls.Labels[k] = v
	}
	return ls
}

type fixedSizeQueue struct {
	sz *remotecommand.TerminalSize
}

func (q *fixedSizeQueue) Next() *remotecommand.TerminalSize {
	defer func() { q.sz = nil }()
	return q.sz
}

var _ remotecommand.TerminalSizeQueue = &fixedSizeQueue{}

type execOpts struct {
	client       *ClusterClient
	app          provision.App
	image        string
	unit         string
	cmds         []string
	eventsOutput io.Writer
	stdout       io.Writer
	stderr       io.Writer
	stdin        io.Reader
	termSize     *remotecommand.TerminalSize
	tty          bool
}

func execCommand(ctx context.Context, opts execOpts) error {
	client := opts.client
	ns, err := client.AppNamespace(ctx, opts.app)
	if err != nil {
		return err
	}
	chosenPod, err := client.CoreV1().Pods(ns).Get(ctx, opts.unit, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(errors.Cause(err)) {
			return &provision.UnitNotFoundError{ID: opts.unit}
		}
		return errors.WithStack(err)
	}
	restCli, err := rest.RESTClientFor(client.restConfig)
	if err != nil {
		return errors.WithStack(err)
	}
	l := labelSetFromMeta(&chosenPod.ObjectMeta)
	if l.AppName() != opts.app.GetName() {
		return errors.Errorf("pod %q do not belong to app %q", chosenPod.Name, l.AppName())
	}
	containerName := chosenPod.Spec.Containers[0].Name
	req := restCli.Post().
		Resource("pods").
		Name(chosenPod.Name).
		Namespace(ns).
		SubResource("exec").
		Param("container", containerName)
	req.VersionedParams(&apiv1.PodExecOptions{
		Container: containerName,
		Command:   opts.cmds,
		Stdin:     opts.stdin != nil,
		Stdout:    true,
		Stderr:    true,
		TTY:       opts.tty,
	}, scheme.ParameterCodec)
	exec, err := keepAliveSpdyExecutor(client.restConfig, "POST", req.URL())
	if err != nil {
		return errors.WithStack(err)
	}
	var sizeQueue remotecommand.TerminalSizeQueue
	if opts.termSize != nil {
		sizeQueue = &fixedSizeQueue{
			sz: opts.termSize,
		}
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:             opts.stdin,
		Stdout:            opts.stdout,
		Stderr:            opts.stderr,
		Tty:               opts.tty,
		TerminalSizeQueue: sizeQueue,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type runSinglePodArgs struct {
	client       *ClusterClient
	eventsOutput io.Writer
	stdout       io.Writer
	stderr       io.Writer
	stdin        io.Reader
	termSize     *remotecommand.TerminalSize
	labels       *provision.LabelSet
	cmds         []string
	envs         []apiv1.EnvVar
	name         string
	image        string
	app          provision.App
}

func runPod(ctx context.Context, args runSinglePodArgs) error {
	err := ensureNamespaceForApp(ctx, args.client, args.app)
	if err != nil {
		return err
	}
	err = ensureServiceAccountForApp(ctx, args.client, args.app)
	if err != nil {
		return err
	}
	nodeSelector := provision.NodeLabels(provision.NodeLabelsOpts{
		Pool:   args.app.GetPool(),
		Prefix: tsuruLabelPrefix,
	}).ToNodeByPoolSelector()
	pullSecrets, err := getImagePullSecrets(ctx, args.client, args.image)
	if err != nil {
		return err
	}
	var tty bool
	if args.stdin == nil {
		args.cmds = append([]string{"sh", "-c", "cat >/dev/null && exec $0 \"$@\""}, args.cmds...)
	} else {
		tty = true
	}
	ns, err := args.client.AppNamespace(ctx, args.app)
	if err != nil {
		return err
	}
	pod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      args.name,
			Namespace: ns,
			Labels:    args.labels.ToLabels(),
		},
		Spec: apiv1.PodSpec{
			ImagePullSecrets:   pullSecrets,
			ServiceAccountName: serviceAccountNameForApp(args.app),
			NodeSelector:       nodeSelector,
			RestartPolicy:      apiv1.RestartPolicyNever,
			Containers: []apiv1.Container{
				{
					Name:      args.name,
					Image:     args.image,
					Command:   args.cmds,
					Env:       args.envs,
					Stdin:     true,
					StdinOnce: true,
					TTY:       tty,
				},
			},
		},
	}

	var initialResource string
	if args.eventsOutput != nil {
		var events *apiv1.EventList
		events, err = args.client.CoreV1().Events(ns).List(ctx, listOptsForResourceEvent("Pod", pod.Name))
		if err != nil {
			return errors.WithStack(err)
		}
		initialResource = events.ResourceVersion
	}

	_, err = args.client.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	defer cleanupPod(tsuruNet.WithoutCancel(ctx), args.client, pod.Name, ns)

	if args.eventsOutput != nil {
		var closeFn func()
		closeFn, err = logPodEvents(ctx, args.client, initialResource, pod.Name, ns, args.eventsOutput)
		if err != nil {
			return err
		}
		defer closeFn()
	}

	kubeConf := getKubeConfig()
	multiErr := tsuruErrors.NewMultiError()
	tctx, cancel := context.WithTimeout(ctx, kubeConf.PodRunningTimeout)
	err = waitForPod(tctx, args.client, pod, ns, true)
	cancel()
	if err != nil {
		multiErr.Add(err)
	}
	if args.stdin == nil {
		args.stdin = bytes.NewBufferString(".")
	}
	err = doAttach(ctx, args.client, args.stdin, args.stdout, args.stderr, pod.Name, args.name, tty, args.termSize, ns)
	if err != nil {
		multiErr.Add(errors.WithStack(err))
	}
	if multiErr.Len() > 0 {
		return multiErr
	}
	tctx, cancel = context.WithTimeout(ctx, kubeConf.PodReadyTimeout)
	defer cancel()
	return waitForPod(tctx, args.client, pod, ns, false)
}

func (p *kubernetesProvisioner) getNodeByAddr(ctx context.Context, client *ClusterClient, address string) (*apiv1.Node, error) {
	address = tsuruNet.URLToHost(address)
	node, err := client.CoreV1().Nodes().Get(ctx, address, metav1.GetOptions{})
	if err == nil {
		return node, nil
	}
	if !k8sErrors.IsNotFound(err) {
		return nil, errors.WithStack(err)
	}
	node = nil
	controller, err := getClusterController(p, client)
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
nodesloop:
	for _, n := range nodeList {
		for _, addr := range n.Status.Addresses {
			if addr.Type == apiv1.NodeInternalIP && addr.Address == address {
				node = n.DeepCopy()
				break nodesloop
			}
		}
	}
	if node != nil {
		return node, nil
	}
	return nil, provision.ErrNodeNotFound
}

func updateAppNamespace(ctx context.Context, client *ClusterClient, appName, namespaceName string) error {
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	oldAppCR, err := getAppCR(ctx, client, appName)
	if err != nil {
		return err
	}
	if oldAppCR.Spec.NamespaceName == namespaceName {
		return nil
	}
	oldAppCR.Spec.NamespaceName = namespaceName
	_, err = tclient.TsuruV1().Apps(client.Namespace()).Update(ctx, oldAppCR, metav1.UpdateOptions{})
	return err
}

func getAppCR(ctx context.Context, client *ClusterClient, appName string) (*tsuruv1.App, error) {
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return nil, err
	}
	return tclient.TsuruV1().Apps(client.Namespace()).Get(ctx, appName, metav1.GetOptions{})
}

func waitForContainerFinished(ctx context.Context, client *ClusterClient, podName, containerName, namespace string) error {
	return waitFor(ctx, func() (bool, error) {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				return true, nil
			}
			return true, errors.WithStack(err)
		}
		switch pod.Status.Phase {
		case apiv1.PodSucceeded:
			fallthrough
		case apiv1.PodFailed:
			return true, nil
		}
		for _, contStatus := range pod.Status.ContainerStatuses {
			if contStatus.Name == containerName && contStatus.State.Terminated != nil {
				return true, nil
			}
		}
		return false, nil
	}, nil)
}

func isPodReady(pod *apiv1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == apiv1.PodReady && cond.Status != apiv1.ConditionTrue {
			return false
		}
	}
	for _, contStatus := range pod.Status.ContainerStatuses {
		if !contStatus.Ready {
			return false
		}
	}
	return true
}
