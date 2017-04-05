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

func deployPodNameForApp(a provision.App) string {
	return fmt.Sprintf("%s-deploy", a.GetName())
}

func execCommandPodNameForApp(a provision.App) string {
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

func notReadyPodEvents(client *Cluster, a provision.App, process string) ([]string, error) {
	l, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:     a,
		Process: process,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	pods, err := client.Core().Pods(client.namespace()).List(v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(l.ToSelector())).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var podsForEvts []string
podsLoop:
	for _, pod := range pods.Items {
		for _, cond := range pod.Status.Conditions {
			if cond.Type == v1.PodReady && cond.Status != v1.ConditionTrue {
				podsForEvts = append(podsForEvts, pod.Name)
				continue podsLoop
			}
		}
	}
	var messages []string
	for _, podName := range podsForEvts {
		eventsInterface := client.Core().Events(client.namespace())
		ns := client.namespace()
		selector := eventsInterface.GetFieldSelector(&podName, &ns, nil, nil)
		options := v1.ListOptions{FieldSelector: selector.String()}
		var events *v1.EventList
		events, err = eventsInterface.List(options)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if len(events.Items) == 0 {
			continue
		}
		lastEvt := events.Items[len(events.Items)-1]
		messages = append(messages, fmt.Sprintf("Pod %s: %s - %s", podName, lastEvt.Reason, lastEvt.Message))
	}
	return messages, nil
}

func waitForPod(client *Cluster, podName string, returnOnRunning bool, timeout time.Duration) error {
	return waitFor(timeout, func() (bool, error) {
		pod, err := client.Core().Pods(client.namespace()).Get(podName)
		if err != nil {
			return true, errors.WithStack(err)
		}
		if pod.Status.Phase == v1.PodPending {
			return false, nil
		}
		switch pod.Status.Phase {
		case v1.PodRunning:
			if !returnOnRunning {
				return false, nil
			}
		case v1.PodUnknown:
			fallthrough
		case v1.PodFailed:
			eventsInterface := client.Core().Events(client.namespace())
			ns := client.namespace()
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

func cleanupPods(client *Cluster, opts v1.ListOptions) error {
	pods, err := client.Core().Pods(client.namespace()).List(opts)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pod := range pods.Items {
		err = client.Core().Pods(client.namespace()).Delete(pod.Name, &v1.DeleteOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func cleanupReplicas(client *Cluster, opts v1.ListOptions) error {
	replicas, err := client.Extensions().ReplicaSets(client.namespace()).List(opts)
	if err != nil {
		return errors.WithStack(err)
	}
	falseVar := false
	for _, replica := range replicas.Items {
		err = client.Extensions().ReplicaSets(client.namespace()).Delete(replica.Name, &v1.DeleteOptions{
			OrphanDependents: &falseVar,
		})
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return cleanupPods(client, opts)
}

func cleanupDeployment(client *Cluster, a provision.App, process string) error {
	depName := deploymentNameForApp(a, process)
	falseVar := false
	err := client.Extensions().Deployments(client.namespace()).Delete(depName, &v1.DeleteOptions{
		OrphanDependents: &falseVar,
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
	return cleanupReplicas(client, v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(l.ToSelector())).String(),
	})
}

func cleanupDaemonSet(client *Cluster, name, pool string) error {
	dsName := daemonSetName(name, pool)
	falseVar := false
	err := client.Extensions().DaemonSets(client.namespace()).Delete(dsName, &v1.DeleteOptions{
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

func cleanupPod(client *Cluster, podName string) error {
	err := client.Core().Pods(client.namespace()).Delete(podName, &v1.DeleteOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return nil
}

func podsFromNode(client *Cluster, nodeName string) ([]v1.Pod, error) {
	podList, err := client.Core().Pods(client.namespace()).List(v1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"spec.nodeName": nodeName,
		}).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return podList.Items, nil
}

func getServicePort(client *Cluster, srvName string) (int32, error) {
	srv, err := client.Core().Services(client.namespace()).Get(srvName)
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
	client, err := clusterForPool(opts.app.GetPool())
	if err != nil {
		return err
	}
	var chosenPod *v1.Pod
	if opts.unit != "" {
		chosenPod, err = client.Core().Pods(client.namespace()).Get(opts.unit)
		if err != nil {
			if k8sErrors.IsNotFound(errors.Cause(err)) {
				return &provision.UnitNotFoundError{ID: opts.unit}
			}
			return errors.WithStack(err)
		}
	} else {
		var l *provision.LabelSet
		l, err = provision.ServiceLabels(provision.ServiceLabelsOpts{
			App: opts.app,
			ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
				Prefix:      tsuruLabelPrefix,
				Provisioner: provisionerName,
			},
		})
		if err != nil {
			return errors.WithStack(err)
		}
		var pods *v1.PodList
		pods, err = client.Core().Pods(client.namespace()).List(v1.ListOptions{
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
		Namespace(client.namespace()).
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
