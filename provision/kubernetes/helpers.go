// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"k8s.io/api/apps/v1beta2"
	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	tsuruLabelPrefix       = "tsuru.io/"
	tsuruInProgressTaint   = tsuruLabelPrefix + "inprogress"
	tsuruNodeDisabledTaint = tsuruLabelPrefix + "disabled"
	replicaDepRevision     = "deployment.kubernetes.io/revision"
	kubeKindReplicaSet     = "ReplicaSet"
)

var kubeNameRegex = regexp.MustCompile(`(?i)[^a-z0-9.-]`)

func serviceAccountNameForApp(a provision.App) string {
	name := strings.ToLower(kubeNameRegex.ReplaceAllString(a.GetName(), "-"))
	return fmt.Sprintf("app-%s", name)
}

func serviceAccountNameForNodeContainer(nodeContainer nodecontainer.NodeContainerConfig) string {
	name := strings.ToLower(kubeNameRegex.ReplaceAllString(nodeContainer.Name, "-"))
	return fmt.Sprintf("node-container-%s", name)
}

func deploymentNameForApp(a provision.App, process string) string {
	name := strings.ToLower(kubeNameRegex.ReplaceAllString(a.GetName(), "-"))
	process = strings.ToLower(kubeNameRegex.ReplaceAllString(process, "-"))
	return fmt.Sprintf("%s-%s", name, process)
}

func headlessServiceNameForApp(a provision.App, process string) string {
	name := strings.ToLower(kubeNameRegex.ReplaceAllString(a.GetName(), "-"))
	process = strings.ToLower(kubeNameRegex.ReplaceAllString(process, "-"))
	return fmt.Sprintf("%s-%s-units", name, process)
}

func deployPodNameForApp(a provision.App) (string, error) {
	version, err := image.AppCurrentImageVersion(a.GetName())
	if err != nil {
		return "", errors.WithMessage(err, "failed to retrieve app current image version")
	}
	name := strings.ToLower(kubeNameRegex.ReplaceAllString(a.GetName(), "-"))
	return fmt.Sprintf("%s-%s-deploy", name, version), nil
}

func buildPodNameForApp(a provision.App, suffix string) (string, error) {
	version, err := image.AppCurrentImageVersion(a.GetName())
	if err != nil {
		return "", errors.WithMessage(err, "failed to retrieve app current image version")
	}
	name := strings.ToLower(kubeNameRegex.ReplaceAllString(a.GetName(), "-"))
	if suffix != "" {
		return fmt.Sprintf("%s-%s-build-%s", name, version, suffix), nil
	}
	return fmt.Sprintf("%s-%s-build", name, version), nil
}

func execCommandPodNameForApp(a provision.App) string {
	name := strings.ToLower(kubeNameRegex.ReplaceAllString(a.GetName(), "-"))
	return fmt.Sprintf("%s-isolated-run", name)
}

func daemonSetName(name, pool string) string {
	name = strings.ToLower(kubeNameRegex.ReplaceAllString(name, "-"))
	pool = strings.ToLower(kubeNameRegex.ReplaceAllString(pool, "-"))
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
	registry = strings.ToLower(kubeNameRegex.ReplaceAllString(registry, "-"))
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
	replicaSets, err := client.AppsV1beta2().ReplicaSets(ns).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToSelector())).String(),
	})
	if err != nil {
		return false, errors.WithStack(err)
	}
	var replica *v1beta2.ReplicaSet
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

func notReadyPodEvents(client *ClusterClient, a provision.App, process string) ([]string, error) {
	pods, err := podsForAppProcess(client, a, process)
	if err != nil {
		return nil, err
	}
	var podsForEvts []*apiv1.Pod
podsLoop:
	for i, pod := range pods.Items {
		for _, cond := range pod.Status.Conditions {
			if cond.Type == apiv1.PodReady && cond.Status != apiv1.ConditionTrue {
				podsForEvts = append(podsForEvts, &pods.Items[i])
				continue podsLoop
			}
		}
	}
	var messages []string
	ns, err := client.AppNamespace(a)
	if err != nil {
		return nil, err
	}
	for _, pod := range podsForEvts {
		err = newInvalidPodPhaseError(client, pod, ns)
		messages = append(messages, fmt.Sprintf("Pod %s: %v", pod.Name, err))
	}
	return messages, nil
}

func waitForPodContainersRunning(ctx context.Context, client *ClusterClient, podName, namespace string) error {
	return waitFor(ctx, func() (bool, error) {
		err := waitForPod(ctx, client, podName, namespace, true)
		if err != nil {
			return true, errors.WithStack(err)
		}
		pod, err := client.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
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
		pod, err := client.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		return newInvalidPodPhaseError(client, pod, namespace)
	})
}

func newInvalidPodPhaseError(client *ClusterClient, pod *apiv1.Pod, namespace string) error {
	phaseWithMsg := fmt.Sprintf("%q", pod.Status.Phase)
	if pod.Status.Message != "" {
		phaseWithMsg = fmt.Sprintf("%s(%q)", phaseWithMsg, pod.Status.Message)
	}
	retErr := errors.Errorf("invalid pod phase %s", phaseWithMsg)
	eventsInterface := client.CoreV1().Events(namespace)
	selector := eventsInterface.GetFieldSelector(&pod.Name, &namespace, nil, nil)
	options := metav1.ListOptions{FieldSelector: selector.String()}
	events, err := eventsInterface.List(options)
	if err == nil && len(events.Items) > 0 {
		lastEvt := events.Items[len(events.Items)-1]
		retErr = errors.Errorf("%v - last event: %s", retErr, lastEvt.Message)
	}
	return retErr
}

func waitForPod(ctx context.Context, client *ClusterClient, podName, namespace string, returnOnRunning bool) error {
	return waitFor(ctx, func() (bool, error) {
		pod, err := client.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
		if err != nil {
			return true, errors.WithStack(err)
		}
		if pod.Status.Phase == apiv1.PodPending {
			return false, nil
		}
		switch pod.Status.Phase {
		case apiv1.PodRunning:
			if !returnOnRunning {
				return false, nil
			}
		case apiv1.PodUnknown:
			fallthrough
		case apiv1.PodFailed:
			return true, newInvalidPodPhaseError(client, pod, namespace)
		}
		return true, nil
	}, func() error {
		pod, err := client.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
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
	replicas, err := client.AppsV1beta2().ReplicaSets(namespace).List(opts)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, replica := range replicas.Items {
		err = client.AppsV1beta2().ReplicaSets(namespace).Delete(replica.Name, &metav1.DeleteOptions{
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
	err = client.AppsV1beta2().Deployments(ns).Delete(depName, &metav1.DeleteOptions{
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
	err := client.AppsV1beta2().DaemonSets(ns).Delete(dsName, &metav1.DeleteOptions{
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

func getServicePort(client *ClusterClient, srvName, namespace string) (int32, error) {
	srv, err := client.CoreV1().Services(namespace).Get(srvName, metav1.GetOptions{})
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if len(srv.Spec.Ports) == 0 {
		return 0, nil
	}
	return srv.Spec.Ports[0].NodePort, nil
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
	client   *ClusterClient
	app      provision.App
	unit     string
	cmds     []string
	stdout   io.Writer
	stderr   io.Writer
	stdin    io.Reader
	termSize *remotecommand.TerminalSize
	tty      bool
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
	var sizeQueue *fixedSizeQueue
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
	client   *ClusterClient
	stdout   io.Writer
	stderr   io.Writer
	stdin    io.Reader
	termSize *remotecommand.TerminalSize
	labels   *provision.LabelSet
	cmds     []string
	envs     []apiv1.EnvVar
	name     string
	image    string
	app      provision.App
}

func runPod(args runSinglePodArgs) error {
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
	_, err = args.client.CoreV1().Pods(ns).Create(pod)
	if err != nil {
		return errors.WithStack(err)
	}
	defer cleanupPod(args.client, pod.Name, ns)
	kubeConf := getKubeConfig()
	multiErr := tsuruErrors.NewMultiError()
	ctx, cancel := context.WithTimeout(context.Background(), kubeConf.PodRunningTimeout)
	err = waitForPod(ctx, args.client, pod.Name, ns, true)
	cancel()
	if err != nil {
		multiErr.Add(err)
	}
	if args.stdin == nil {
		args.stdin = bytes.NewBufferString(".")
	}
	err = doAttach(args.client, args.stdin, args.stdout, args.stderr, pod.Name, args.name, tty, args.termSize, ns)
	if err != nil {
		multiErr.Add(errors.WithStack(err))
	}
	if multiErr.Len() > 0 {
		return multiErr
	}
	ctx, cancel = context.WithTimeout(context.Background(), kubeConf.PodReadyTimeout)
	defer cancel()
	return waitForPod(ctx, args.client, pod.Name, ns, false)
}

func getNodeByAddr(client *ClusterClient, address string) (*apiv1.Node, error) {
	address = tsuruNet.URLToHost(address)
	node, err := client.CoreV1().Nodes().Get(address, metav1.GetOptions{})
	if err == nil {
		return node, nil
	}
	if !k8sErrors.IsNotFound(err) {
		return nil, err
	}
	node = nil
	nodeList, err := client.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
nodesloop:
	for i, n := range nodeList.Items {
		for _, addr := range n.Status.Addresses {
			if addr.Type == apiv1.NodeInternalIP && addr.Address == address {
				node = &nodeList.Items[i]
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
