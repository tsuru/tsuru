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
	"strings"
	"time"

	"github.com/pkg/errors"
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
	replicaDepRevision        = "deployment.kubernetes.io/revision"
	kubeKindReplicaSet        = "ReplicaSet"
	kubeLabelNameMaxLen       = 55
)

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

func deploymentNameForApp(a provision.App, process string) string {
	return appProcessName(a, process)
}

func headlessServiceNameForApp(a provision.App, process string) string {
	return fmt.Sprintf("%s-units", appProcessName(a, process))
}

func deployPodNameForApp(a provision.App, version appTypes.AppVersion) string {
	name := validKubeName(a.GetName())
	return fmt.Sprintf("%s-v%d-deploy", name, version.Version())
}

func buildPodNameForApp(a provision.App, version appTypes.AppVersion) string {
	name := validKubeName(a.GetName())
	return fmt.Sprintf("%s-v%d-build", name, version.Version())
}

func appLabelForApp(a provision.App, process string) string {
	return appProcessName(a, process)
}

func appProcessName(a provision.App, process string) string {
	name := validKubeName(a.GetName())
	process = validKubeName(process)
	label := fmt.Sprintf("%s-%s", name, process)
	if len(label) > kubeLabelNameMaxLen {
		h := sha256.New()
		h.Write([]byte(process))
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

func podsForAppProcess(client *ClusterClient, a provision.App, process string) (*apiv1.PodList, error) {
	labelOpts := provision.ServiceLabelsOpts{
		App:     a,
		Process: process,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	}
	l, err := provision.ServiceLabels(labelOpts)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var selector map[string]string
	if process == "" {
		selector = l.ToAppSelector()
	} else {
		selector = l.ToSelector()
	}
	ns, err := client.AppNamespace(a)
	if err != nil {
		return nil, err
	}
	podList, err := client.CoreV1().Pods(ns).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(selector)).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return podList, nil
}

func allNewPodsRunning(client *ClusterClient, a provision.App, process string, depRevision string) (bool, error) {
	labelOpts := provision.ServiceLabelsOpts{
		App:     a,
		Process: process,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	}
	ls, err := provision.ServiceLabels(labelOpts)
	if err != nil {
		return false, errors.WithStack(err)
	}
	ns, err := client.AppNamespace(a)
	if err != nil {
		return false, err
	}
	replicaSets, err := client.AppsV1().ReplicaSets(ns).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToSelector())).String(),
	})
	if err != nil {
		return false, errors.WithStack(err)
	}
	var replica *appsv1.ReplicaSet
	for i, rs := range replicaSets.Items {
		if rs.Annotations != nil && rs.Annotations[replicaDepRevision] == depRevision {
			replica = &replicaSets.Items[i]
			break
		}
	}
	if replica == nil {
		return false, nil
	}
	pods, err := podsForAppProcess(client, a, process)
	if err != nil {
		return false, err
	}
	newCount := 0
	for _, pod := range pods.Items {
		newPod := false
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == kubeKindReplicaSet && ref.Name == replica.Name {
				newPod = true
				break
			}
		}
		if !newPod {
			continue
		}
		newCount++
		if pod.Status.Phase != apiv1.PodRunning {
			return false, nil
		}
	}
	return newCount > 0, nil
}

type podErrorMessage struct {
	podName string
	message string
}

func notReadyPodEvents(client *ClusterClient, a provision.App, process string) ([]podErrorMessage, error) {
	pods, err := podsForAppProcess(client, a, process)
	if err != nil {
		return nil, err
	}
	return notReadyPodEventsForPods(client, pods.Items)
}

func notReadyPodEventsForPod(client *ClusterClient, podName, ns string) ([]podErrorMessage, error) {
	pod, err := client.CoreV1().Pods(ns).Get(podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return notReadyPodEventsForPods(client, []apiv1.Pod{*pod})
}

func notReadyPodEventsForPods(client *ClusterClient, pods []apiv1.Pod) ([]podErrorMessage, error) {
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
				messages = append(messages, podErrorMessage{podName: pod.Name, message: msg})
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
			messages = append(messages, podErrorMessage{podName: pod.Name, message: msg})
		}
		lastEvt, err := lastEventForPod(client, &pod)
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
			messages = append(messages, podErrorMessage{podName: pod.Name, message: msg})
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
		pod, err := client.CoreV1().Pods(namespace).Get(origPod.Name, metav1.GetOptions{})
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
		pod, err := client.CoreV1().Pods(namespace).Get(origPod.Name, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		return newInvalidPodPhaseError(client, pod, namespace)
	})
}

func eventsForPod(client *ClusterClient, pod *apiv1.Pod) (*apiv1.EventList, error) {
	eventsInterface := client.CoreV1().Events(pod.Namespace)
	selector := eventsInterface.GetFieldSelector(&pod.Name, &pod.Namespace, nil, nil)
	options := metav1.ListOptions{
		FieldSelector: selector.String(),
	}
	return eventsInterface.List(options)
}

func lastEventForPod(client *ClusterClient, pod *apiv1.Pod) (*apiv1.Event, error) {
	events, err := eventsForPod(client, pod)
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

func newInvalidPodPhaseError(client *ClusterClient, pod *apiv1.Pod, namespace string) error {
	phaseWithMsg := fmt.Sprintf("%q", pod.Status.Phase)
	if pod.Status.Message != "" {
		phaseWithMsg = fmt.Sprintf("%s(%q)", phaseWithMsg, pod.Status.Message)
	}
	retErr := errors.Errorf("invalid pod phase %s", phaseWithMsg)
	lastEvt, err := lastEventForPod(client, pod)
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
		pod, err := client.CoreV1().Pods(namespace).Get(origPod.Name, metav1.GetOptions{})
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
					invalidErr := newInvalidPodPhaseError(client, pod, namespace)
					return true, errors.Wrapf(invalidErr, "unexpected container %q termination: Exit %d - Reason: %q - Message: %q", contStatus.Name, termData.ExitCode, termData.Reason, termData.Message)
				}
			}
			return allDone, nil
		case apiv1.PodUnknown:
			fallthrough
		case apiv1.PodFailed:
			return true, newInvalidPodPhaseError(client, pod, namespace)
		}
		return true, nil
	}, func() error {
		pod, err := client.CoreV1().Pods(namespace).Get(origPod.Name, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		return newInvalidPodPhaseError(client, pod, namespace)
	})
}

func cleanupPods(client *ClusterClient, opts metav1.ListOptions, namespace string) error {
	pods, err := client.CoreV1().Pods(namespace).List(opts)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pod := range pods.Items {
		err = client.CoreV1().Pods(namespace).Delete(pod.Name, &metav1.DeleteOptions{})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
	}
	return nil
}

func propagationPtr(p metav1.DeletionPropagation) *metav1.DeletionPropagation {
	return &p
}

func cleanupReplicas(client *ClusterClient, opts metav1.ListOptions, namespace string) error {
	replicas, err := client.AppsV1().ReplicaSets(namespace).List(opts)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, replica := range replicas.Items {
		err = client.AppsV1().ReplicaSets(namespace).Delete(replica.Name, &metav1.DeleteOptions{
			PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
		})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
	}
	return cleanupPods(client, opts, namespace)
}

func cleanupDeployment(client *ClusterClient, a provision.App, process string) error {
	depName := deploymentNameForApp(a, process)
	ns, err := client.AppNamespace(a)
	if err != nil {
		return err
	}
	err = client.AppsV1().Deployments(ns).Delete(depName, &metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	l, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:     a,
		Process: process,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return err
	}
	return cleanupReplicas(client, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(l.ToSelector())).String(),
	}, ns)
}

func cleanupDaemonSet(client *ClusterClient, name, pool string) error {
	dsName := daemonSetName(name, pool)
	ns := client.PoolNamespace(pool)
	err := client.AppsV1().DaemonSets(ns).Delete(dsName, &metav1.DeleteOptions{
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
	return cleanupPods(client, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToNodeContainerSelector())).String(),
	}, ns)
}

func cleanupPod(client *ClusterClient, podName, namespace string) error {
	noWait := int64(0)
	err := client.CoreV1().Pods(namespace).Delete(podName, &metav1.DeleteOptions{
		GracePeriodSeconds: &noWait,
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		log.Errorf("[cleanupPod] Deferred cleanup action failed %s", err.Error())
		return errors.WithStack(err)
	}
	return nil
}

func podsFromNode(client *ClusterClient, nodeName, labelFilter string) ([]apiv1.Pod, error) {
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
		Do().
		Into(&podList)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return podList.Items, nil
}

func appPodsFromNode(client *ClusterClient, nodeName string) ([]apiv1.Pod, error) {
	l := provision.LabelSet{Prefix: tsuruLabelPrefix}
	l.SetIsService()
	serviceSelector := fields.SelectorFromSet(fields.Set(l.ToIsServiceSelector())).String()
	labelFilter := fmt.Sprintf("%s,%s%s", serviceSelector, tsuruLabelPrefix, provision.LabelAppPool)
	return podsFromNode(client, nodeName, labelFilter)
}

func getServicePort(svcInformer v1informers.ServiceInformer, srvName, namespace string) (int32, error) {
	ports, err := getServicePorts(svcInformer, srvName, namespace)
	if err != nil {
		return 0, err
	}
	if len(ports) > 0 {
		return ports[0], nil
	}
	return 0, nil
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

func labelSetFromMeta(meta *metav1.ObjectMeta) *provision.LabelSet {
	merged := make(map[string]string, len(meta.Labels)+len(meta.Annotations))
	for k, v := range meta.Labels {
		merged[k] = v
	}
	for k, v := range meta.Annotations {
		merged[k] = v
	}
	return &provision.LabelSet{Labels: merged, Prefix: tsuruLabelPrefix}
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

func execCommand(opts execOpts) error {
	client := opts.client
	ns, err := client.AppNamespace(opts.app)
	if err != nil {
		return err
	}
	chosenPod, err := client.CoreV1().Pods(ns).Get(opts.unit, metav1.GetOptions{})
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
	containerName := deploymentNameForApp(opts.app, l.AppProcess())
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
	err := ensureNamespaceForApp(args.client, args.app)
	if err != nil {
		return err
	}
	err = ensureServiceAccountForApp(args.client, args.app)
	if err != nil {
		return err
	}
	nodeSelector := provision.NodeLabels(provision.NodeLabelsOpts{
		Pool:   args.app.GetPool(),
		Prefix: tsuruLabelPrefix,
	}).ToNodeByPoolSelector()
	pullSecrets, err := getImagePullSecrets(args.client, args.image)
	if err != nil {
		return err
	}
	labels, annotations := provision.SplitServiceLabelsAnnotations(args.labels)
	var tty bool
	if args.stdin == nil {
		args.cmds = append([]string{"sh", "-c", "cat >/dev/null && exec $0 \"$@\""}, args.cmds...)
	} else {
		tty = true
	}
	ns, err := args.client.AppNamespace(args.app)
	if err != nil {
		return err
	}
	pod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        args.name,
			Namespace:   ns,
			Labels:      labels.ToLabels(),
			Annotations: annotations.ToLabels(),
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
		events, err = args.client.CoreV1().Events(ns).List(listOptsForPodEvent(pod.Name))
		if err != nil {
			return errors.WithStack(err)
		}
		initialResource = events.ResourceVersion
	}

	_, err = args.client.CoreV1().Pods(ns).Create(pod)
	if err != nil {
		return errors.WithStack(err)
	}
	defer cleanupPod(args.client, pod.Name, ns)

	if args.eventsOutput != nil {
		var closeFn func()
		closeFn, err = logPodEvents(args.client, initialResource, pod.Name, ns, args.eventsOutput)
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

func (p *kubernetesProvisioner) getNodeByAddr(client *ClusterClient, address string) (*apiv1.Node, error) {
	address = tsuruNet.URLToHost(address)
	node, err := client.CoreV1().Nodes().Get(address, metav1.GetOptions{})
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

func updateAppNamespace(client *ClusterClient, appName, namespaceName string) error {
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return err
	}
	oldAppCR, err := getAppCR(client, appName)
	if err != nil {
		return err
	}
	if oldAppCR.Spec.NamespaceName == namespaceName {
		return nil
	}
	oldAppCR.Spec.NamespaceName = namespaceName
	_, err = tclient.TsuruV1().Apps(client.Namespace()).Update(oldAppCR)
	return err
}

func getAppCR(client *ClusterClient, appName string) (*tsuruv1.App, error) {
	tclient, err := TsuruClientForConfig(client.restConfig)
	if err != nil {
		return nil, err
	}
	return tclient.TsuruV1().Apps(client.Namespace()).Get(appName, metav1.GetOptions{})
}

func waitForContainerFinished(ctx context.Context, client *ClusterClient, podName, containerName, namespace string) error {
	return waitFor(ctx, func() (bool, error) {
		pod, err := client.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
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
