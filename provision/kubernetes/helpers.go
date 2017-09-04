// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	remotecommandutil "k8s.io/apimachinery/pkg/util/remotecommand"
	"k8s.io/client-go/kubernetes/scheme"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	tsuruLabelPrefix   = "tsuru.io/"
	replicaDepRevision = "deployment.kubernetes.io/revision"
	kubeKindReplicaSet = "ReplicaSet"
)

var kubeNameRegex = regexp.MustCompile(`(?i)[^a-z0-9.-]`)

func deploymentNameForApp(a provision.App, process string) string {
	name := strings.ToLower(kubeNameRegex.ReplaceAllString(a.GetName(), "-"))
	process = strings.ToLower(kubeNameRegex.ReplaceAllString(process, "-"))
	return fmt.Sprintf("%s-%s", name, process)
}

func deployPodNameForApp(a provision.App) (string, error) {
	version, err := image.AppCurrentImageVersion(a.GetName())
	if err != nil {
		return "", errors.WithMessage(err, "failed to retrieve app current image version")
	}
	name := strings.ToLower(kubeNameRegex.ReplaceAllString(a.GetName(), "-"))
	return fmt.Sprintf("%s-%s-deploy", name, version), nil
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

func waitFor(timeout time.Duration, fn func() (bool, error), onTimeout func() error) error {
	timeoutCh := time.After(timeout)
	for {
		done, err := fn()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-timeoutCh:
			if onTimeout == nil {
				err = errors.Errorf("timeout after %v", timeout)
			} else {
				err = errors.Errorf("timeout after %v: %v", timeout, onTimeout())
			}
			return err
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func podsForAppProcess(client *clusterClient, a provision.App, process string) (*apiv1.PodList, error) {
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
	podList, err := client.Core().Pods(client.Namespace()).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(selector)).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return podList, nil
}

func allNewPodsRunning(client *clusterClient, a provision.App, process string, generation int64) (bool, error) {
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
	replicaSets, err := client.Extensions().ReplicaSets(client.Namespace()).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToSelector())).String(),
	})
	if err != nil {
		return false, errors.WithStack(err)
	}
	generationStr := strconv.Itoa(int(generation))
	var replica *v1beta1.ReplicaSet
	for i, rs := range replicaSets.Items {
		if rs.Annotations != nil && rs.Annotations[replicaDepRevision] == generationStr {
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

func notReadyPodEvents(client *clusterClient, a provision.App, process string) ([]string, error) {
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
	for _, pod := range podsForEvts {
		err = newInvalidPodPhaseError(client, pod)
		messages = append(messages, fmt.Sprintf("Pod %s: %v", pod.Name, err))
	}
	return messages, nil
}

func waitForPodContainersRunning(client *clusterClient, podName string, timeout time.Duration) error {
	return waitFor(timeout, func() (bool, error) {
		err := waitForPod(client, podName, true, timeout)
		if err != nil {
			return true, errors.WithStack(err)
		}
		pod, err := client.Core().Pods(client.Namespace()).Get(podName, metav1.GetOptions{})
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
		pod, err := client.Core().Pods(client.Namespace()).Get(podName, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		return newInvalidPodPhaseError(client, pod)
	})
}

func newInvalidPodPhaseError(client *clusterClient, pod *apiv1.Pod) error {
	phaseWithMsg := fmt.Sprintf("%q", pod.Status.Phase)
	if pod.Status.Message != "" {
		phaseWithMsg = fmt.Sprintf("%s(%q)", phaseWithMsg, pod.Status.Message)
	}
	retErr := errors.Errorf("invalid pod phase %s", phaseWithMsg)
	eventsInterface := client.Core().Events(client.Namespace())
	ns := client.Namespace()
	selector := eventsInterface.GetFieldSelector(&pod.Name, &ns, nil, nil)
	options := metav1.ListOptions{FieldSelector: selector.String()}
	events, err := eventsInterface.List(options)
	if err == nil && len(events.Items) > 0 {
		lastEvt := events.Items[len(events.Items)-1]
		retErr = errors.Errorf("%v - last event: %s", retErr, lastEvt.Message)
	}
	if len(pod.Spec.Containers) > 0 {
		lastLog := int64(100)
		req := client.Core().Pods(client.Namespace()).GetLogs(pod.Name, &apiv1.PodLogOptions{
			Container: pod.Spec.Containers[0].Name,
			TailLines: &lastLog,
		})
		reader, logErr := req.Stream()
		if logErr == nil {
			logContent, _ := ioutil.ReadAll(reader)
			if len(logContent) > 0 {
				retErr = errors.Errorf("%v - log: %s", retErr, string(logContent))
			}
		}
	}
	return retErr
}

func waitForPod(client *clusterClient, podName string, returnOnRunning bool, timeout time.Duration) error {
	return waitFor(timeout, func() (bool, error) {
		pod, err := client.Core().Pods(client.Namespace()).Get(podName, metav1.GetOptions{})
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
			return true, newInvalidPodPhaseError(client, pod)
		}
		return true, nil
	}, func() error {
		pod, err := client.Core().Pods(client.Namespace()).Get(podName, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		return newInvalidPodPhaseError(client, pod)
	})
}

func cleanupPods(client *clusterClient, opts metav1.ListOptions) error {
	pods, err := client.Core().Pods(client.Namespace()).List(opts)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pod := range pods.Items {
		err = client.Core().Pods(client.Namespace()).Delete(pod.Name, &metav1.DeleteOptions{})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
	}
	return nil
}

func propagationPtr(p metav1.DeletionPropagation) *metav1.DeletionPropagation {
	return &p
}

func cleanupReplicas(client *clusterClient, opts metav1.ListOptions) error {
	replicas, err := client.Extensions().ReplicaSets(client.Namespace()).List(opts)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, replica := range replicas.Items {
		err = client.Extensions().ReplicaSets(client.Namespace()).Delete(replica.Name, &metav1.DeleteOptions{
			PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
		})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
	}
	return cleanupPods(client, opts)
}

func cleanupDeployment(client *clusterClient, a provision.App, process string) error {
	depName := deploymentNameForApp(a, process)
	err := client.Extensions().Deployments(client.Namespace()).Delete(depName, &metav1.DeleteOptions{
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
	})
}

func cleanupDaemonSet(client *clusterClient, name, pool string) error {
	dsName := daemonSetName(name, pool)
	err := client.Extensions().DaemonSets(client.Namespace()).Delete(dsName, &metav1.DeleteOptions{
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
	})
}

func cleanupPod(client *clusterClient, podName string) error {
	noWait := int64(0)
	err := client.Core().Pods(client.Namespace()).Delete(podName, &metav1.DeleteOptions{
		GracePeriodSeconds: &noWait,
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return nil
}

func podsFromNode(client *clusterClient, nodeName string) ([]apiv1.Pod, error) {
	podList, err := client.Core().Pods(client.Namespace()).List(metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"spec.nodeName": nodeName,
		}).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return podList.Items, nil
}

func getServicePort(client *clusterClient, srvName string) (int32, error) {
	srv, err := client.Core().Services(client.Namespace()).Get(srvName, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return 0, nil
		}
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
	client, err := clusterForPool(opts.app.GetPool())
	if err != nil {
		return err
	}
	var chosenPod *apiv1.Pod
	if opts.unit != "" {
		chosenPod, err = client.Core().Pods(client.Namespace()).Get(opts.unit, metav1.GetOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(errors.Cause(err)) {
				return &provision.UnitNotFoundError{ID: opts.unit}
			}
			return errors.WithStack(err)
		}
	} else {
		var pods *apiv1.PodList
		pods, err = podsForAppProcess(client, opts.app, "")
		if err != nil {
			return err
		}
		if len(pods.Items) == 0 {
			return provision.ErrEmptyApp
		}
		chosenPod = &pods.Items[0]
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
		Namespace(client.Namespace()).
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
	exec, err := remotecommand.NewExecutor(client.restConfig, "POST", req.URL())
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
		SupportedProtocols: remotecommandutil.SupportedStreamingProtocols,
		Stdin:              opts.stdin,
		Stdout:             opts.stdout,
		Stderr:             opts.stderr,
		Tty:                opts.tty,
		TerminalSizeQueue:  sizeQueue,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type runSinglePodArgs struct {
	client     *clusterClient
	stdout     io.Writer
	labels     *provision.LabelSet
	cmds       []string
	envs       []apiv1.EnvVar
	name       string
	image      string
	pool       string
	dockerSock bool
}

func runPod(args runSinglePodArgs) error {
	nodeSelector := provision.NodeLabels(provision.NodeLabelsOpts{
		Pool:   args.pool,
		Prefix: tsuruLabelPrefix,
	}).ToNodeByPoolSelector()
	pod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      args.name,
			Namespace: args.client.Namespace(),
			Labels:    args.labels.ToLabels(),
		},
		Spec: apiv1.PodSpec{
			NodeSelector:  nodeSelector,
			RestartPolicy: apiv1.RestartPolicyNever,
			Containers: []apiv1.Container{
				{
					Name:    args.name,
					Image:   args.image,
					Command: args.cmds,
					Env:     args.envs,
				},
			},
		},
	}
	if args.dockerSock {
		pod.Spec.Volumes = []apiv1.Volume{
			{
				Name: "dockersock",
				VolumeSource: apiv1.VolumeSource{
					HostPath: &apiv1.HostPathVolumeSource{
						Path: dockerSockPath,
					},
				},
			},
		}
		pod.Spec.Containers[0].VolumeMounts = []apiv1.VolumeMount{
			{Name: "dockersock", MountPath: dockerSockPath},
		}
	}
	_, err := args.client.Core().Pods(args.client.Namespace()).Create(pod)
	if err != nil {
		return errors.WithStack(err)
	}
	defer cleanupPod(args.client, pod.Name)
	kubeConf := getKubeConfig()
	multiErr := tsuruErrors.NewMultiError()
	err = waitForPod(args.client, pod.Name, true, kubeConf.PodRunningTimeout)
	if err != nil {
		multiErr.Add(err)
	}
	err = args.client.SetTimeout(kubeConf.PodRunningTimeout)
	if err != nil {
		multiErr.Add(errors.WithStack(err))
		return multiErr
	}
	req := args.client.Core().Pods(args.client.Namespace()).GetLogs(pod.Name, &apiv1.PodLogOptions{
		Follow:    true,
		Container: args.name,
	})
	reader, err := req.Stream()
	if err != nil {
		multiErr.Add(errors.WithStack(err))
		return multiErr
	}
	defer reader.Close()
	_, err = io.Copy(args.stdout, reader)
	if err != nil && err != io.EOF {
		multiErr.Add(errors.WithStack(err))
		return multiErr
	}
	return multiErr.ToError()
}

func waitNodeReady(client *clusterClient, addr string, timeout time.Duration) (*apiv1.Node, error) {
	var node *apiv1.Node
	waitErr := waitFor(timeout, func() (bool, error) {
		var err error
		node, err = client.Core().Nodes().Get(addr, metav1.GetOptions{})
		if err != nil {
			return true, errors.WithStack(err)
		}
		for _, cond := range node.Status.Conditions {
			if cond.Type == apiv1.NodeReady && cond.Status == apiv1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	}, func() error {
		var err error
		node, err = client.Core().Nodes().Get(addr, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		return errors.Errorf("invalid node conditions for %q: %#v", addr, node.Status.Conditions)
	})
	return node, waitErr
}

// Hack until Kubernetes 1.7 is released to ensure daemonsets are scheduled.
// See https://github.com/kubernetes/kubernetes/pull/45649
func refreshNodeTaints(client *clusterClient, addr string) {
	node, err := waitNodeReady(client, addr, 30*time.Minute)
	if err != nil {
		log.Errorf("error waiting for node ready: %v", err)
	}
	tsuruTaintKey := "tsuru-refresh-node-container"
	tsuruTempTaint := apiv1.Taint{
		Key:    tsuruTaintKey,
		Value:  "true",
		Effect: apiv1.TaintEffectNoSchedule,
	}
	node.Spec.Taints = append(node.Spec.Taints, tsuruTempTaint)
	node, err = client.Core().Nodes().Update(node)
	if err != nil {
		log.Errorf("unable to add node taint %q: %v", tsuruTaintKey, err)
	}
	for i := 0; i < len(node.Spec.Taints); i++ {
		if node.Spec.Taints[i].Key == tsuruTaintKey {
			node.Spec.Taints[i] = node.Spec.Taints[len(node.Spec.Taints)-1]
			node.Spec.Taints = node.Spec.Taints[:len(node.Spec.Taints)-1]
			i--
		}
	}
	_, err = client.Core().Nodes().Update(node)
	if err != nil {
		log.Errorf("unable to remove node taint %q: %v", tsuruTaintKey, err)
	}
}
