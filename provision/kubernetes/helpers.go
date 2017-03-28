// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"io"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	k8sErrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/fields"
	"k8s.io/client-go/pkg/labels"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/client/unversioned/remotecommand"
	remotecommandserver "k8s.io/kubernetes/pkg/kubelet/server/remotecommand"
	"k8s.io/kubernetes/pkg/util/term"
)

const (
	tsuruLabelPrefix = "tsuru.io/"
)

func deploymentNameForApp(a provision.App, process string) string {
	return fmt.Sprintf("%s-%s", a.GetName(), process)
}

func deployJobNameForApp(a provision.App) string {
	return fmt.Sprintf("%s-deploy", a.GetName())
}

func execCommandJobNameForApp(a provision.App) string {
	return fmt.Sprintf("%s-isolated-run", a.GetName())
}

func daemonSetName(name, pool string) string {
	if pool == "" {
		return fmt.Sprintf("node-container-%s-all", name)
	}
	return fmt.Sprintf("node-container-%s-pool-%s", name, pool)
}

func waitFor(timeout time.Duration, fn func() (bool, error)) error {
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
			return errors.Errorf("timeout after %v", timeout)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func waitForPodStart(client kubernetes.Interface, podName string, timeout time.Duration) error {
	return waitFor(timeout, func() (bool, error) {
		pod, err := client.Core().Pods(tsuruNamespace).Get(podName)
		if err != nil {
			return true, err
		}
		if pod.Status.Phase == v1.PodPending {
			return false, nil
		}
		switch pod.Status.Phase {
		case v1.PodUnknown:
			fallthrough
		case v1.PodFailed:
			eventsInterface := client.Core().Events(tsuruNamespace)
			ns := tsuruNamespace
			selector := eventsInterface.GetFieldSelector(&podName, &ns, nil, nil)
			options := v1.ListOptions{FieldSelector: selector.String()}
			var events *v1.EventList
			events, err = eventsInterface.List(options)
			if err != nil {
				return true, errors.Wrapf(err, "error listing pod %q events invalid phase %q", podName, pod.Status.Phase)
			}
			if len(events.Items) == 0 {
				return true, errors.Errorf("invalid pod phase %q", pod.Status.Phase)
			}
			lastEvt := events.Items[len(events.Items)-1]
			return true, errors.Errorf("invalid pod phase %q: %s", pod.Status.Phase, lastEvt.Message)
		}
		return true, nil
	})
}

func waitForJobContainerRunning(client kubernetes.Interface, matchLabels map[string]string, container string, timeout time.Duration) (string, error) {
	var name string
	err := waitFor(timeout, func() (bool, error) {
		pods, err := client.Core().Pods(tsuruNamespace).List(v1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set(matchLabels)).String(),
		})
		if err != nil {
			return false, errors.WithStack(err)
		}
		if len(pods.Items) == 0 {
			return false, nil
		}
		for _, pod := range pods.Items {
			name = pod.Name
			for _, contStatus := range pod.Status.ContainerStatuses {
				if contStatus.Name == container && contStatus.State.Running != nil {
					return true, nil
				}
			}
		}
		return false, nil
	})
	return name, err
}

func waitForJob(client kubernetes.Interface, jobName string, timeout time.Duration) error {
	return waitFor(timeout, func() (bool, error) {
		job, err := client.Batch().Jobs(tsuruNamespace).Get(jobName)
		if err != nil {
			return false, errors.WithStack(err)
		}
		if job.Status.Failed > 0 {
			return false, errors.Errorf("job %s failed: %#v", jobName, job.Status.Conditions)
		}
		if job.Status.Succeeded == 1 {
			return true, nil
		}
		return false, nil
	})
}

func cleanupPods(client kubernetes.Interface, opts v1.ListOptions) error {
	pods, err := client.Core().Pods(tsuruNamespace).List(opts)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pod := range pods.Items {
		err = client.Core().Pods(tsuruNamespace).Delete(pod.Name, &v1.DeleteOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func cleanupReplicas(client kubernetes.Interface, opts v1.ListOptions) error {
	replicas, err := client.Extensions().ReplicaSets(tsuruNamespace).List(opts)
	if err != nil {
		return errors.WithStack(err)
	}
	falseVar := false
	for _, replica := range replicas.Items {
		err = client.Extensions().ReplicaSets(tsuruNamespace).Delete(replica.Name, &v1.DeleteOptions{
			OrphanDependents: &falseVar,
		})
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return cleanupPods(client, opts)
}

func cleanupDeployment(client kubernetes.Interface, a provision.App, process string) error {
	depName := deploymentNameForApp(a, process)
	falseVar := false
	err := client.Extensions().Deployments(tsuruNamespace).Delete(depName, &v1.DeleteOptions{
		OrphanDependents: &falseVar,
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	l, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:         a,
		Process:     process,
		Provisioner: provisionerName,
		Prefix:      tsuruLabelPrefix,
	})
	if err != nil {
		return err
	}
	return cleanupReplicas(client, v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(l.ToSelector())).String(),
	})
}

func cleanupDaemonSet(client kubernetes.Interface, name, pool string) error {
	dsName := daemonSetName(name, pool)
	falseVar := false
	err := client.Extensions().DaemonSets(tsuruNamespace).Delete(dsName, &v1.DeleteOptions{
		OrphanDependents: &falseVar,
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
	return cleanupPods(client, v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToNodeContainerSelector())).String(),
	})
}

func cleanupJob(client kubernetes.Interface, jobName string) error {
	falseVar := false
	err := client.Batch().Jobs(tsuruNamespace).Delete(jobName, &v1.DeleteOptions{
		OrphanDependents: &falseVar,
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return cleanupPods(client, v1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
}

func cleanupPod(client kubernetes.Interface, podName string) error {
	err := client.Core().Pods(tsuruNamespace).Delete(podName, &v1.DeleteOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return nil
}

func podsFromNode(client kubernetes.Interface, nodeName string) ([]v1.Pod, error) {
	podList, err := client.Core().Pods(tsuruNamespace).List(v1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"spec.nodeName": nodeName,
		}).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return podList.Items, nil
}

func getServicePort(client kubernetes.Interface, srvName string) (int32, error) {
	srv, err := client.Core().Services(tsuruNamespace).Get(srvName)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if len(srv.Spec.Ports) == 0 {
		return 0, nil
	}
	return srv.Spec.Ports[0].NodePort, nil
}

func labelSetFromMeta(meta *v1.ObjectMeta) *provision.LabelSet {
	merged := meta.Labels
	for k, v := range meta.Annotations {
		merged[k] = v
	}
	return &provision.LabelSet{Labels: merged, Prefix: tsuruLabelPrefix}
}

type fixedSizeQueue struct {
	sz *term.Size
}

func (q *fixedSizeQueue) Next() *term.Size {
	defer func() { q.sz = nil }()
	return q.sz
}

var _ term.TerminalSizeQueue = &fixedSizeQueue{}

type execOpts struct {
	app      provision.App
	unit     string
	cmds     []string
	stdout   io.Writer
	stderr   io.Writer
	stdin    io.Reader
	termSize *term.Size
	tty      bool
}

func execCommand(opts execOpts) error {
	client, cfg, err := getClusterClientWithCfg()
	if err != nil {
		return err
	}
	var chosenPod *v1.Pod
	if opts.unit != "" {
		chosenPod, err = client.Core().Pods(tsuruNamespace).Get(opts.unit)
		if err != nil {
			if k8sErrors.IsNotFound(errors.Cause(err)) {
				return &provision.UnitNotFoundError{ID: opts.unit}
			}
			return errors.WithStack(err)
		}
	} else {
		var l *provision.LabelSet
		l, err = provision.ServiceLabels(provision.ServiceLabelsOpts{
			App:         opts.app,
			Provisioner: provisionerName,
			Prefix:      tsuruLabelPrefix,
		})
		if err != nil {
			return errors.WithStack(err)
		}
		var pods *v1.PodList
		pods, err = client.Core().Pods(tsuruNamespace).List(v1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set(l.ToAppSelector())).String(),
		})
		if err != nil {
			return errors.WithStack(err)
		}
		if len(pods.Items) == 0 {
			return provision.ErrEmptyApp
		}
		chosenPod = &pods.Items[0]
	}
	restCli, err := rest.RESTClientFor(cfg)
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
		Namespace(tsuruNamespace).
		SubResource("exec").
		Param("container", containerName)
	req.VersionedParams(&api.PodExecOptions{
		Container: containerName,
		Command:   opts.cmds,
		Stdin:     opts.stdin != nil,
		Stdout:    true,
		Stderr:    true,
		TTY:       opts.tty,
	}, api.ParameterCodec)
	exec, err := remotecommand.NewExecutor(cfg, "POST", req.URL())
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
		SupportedProtocols: remotecommandserver.SupportedStreamingProtocols,
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
