// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"k8s.io/client-go/kubernetes"
	k8sErrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/fields"
	"k8s.io/client-go/pkg/labels"
)

func deploymentNameForApp(a provision.App, process string) string {
	return fmt.Sprintf("%s-%s", a.GetName(), process)
}

func deployJobNameForApp(a provision.App) string {
	return fmt.Sprintf("%s-deploy", a.GetName())
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
	if err != nil {
		return errors.WithStack(err)
	}
	l, err := podLabels(a, process, "", 0)
	if err != nil {
		return err
	}
	return cleanupReplicas(client, v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(l.ToSelector())).String(),
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
