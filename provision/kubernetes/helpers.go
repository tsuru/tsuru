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

func waitForPodRunning(client kubernetes.Interface, jobName string, container string, timeout time.Duration) (string, error) {
	var name string
	err := waitFor(timeout, func() (bool, error) {
		pods, err := client.Core().Pods(tsuruNamespace).List(v1.ListOptions{
			LabelSelector: fmt.Sprintf("job-name=%s", jobName),
		})
		if err != nil {
			return false, err
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
	if err != nil {
		return "", err
	}
	return name, nil
}

func waitForJob(client kubernetes.Interface, jobName string, timeout time.Duration, waitDelete bool) error {
	return waitFor(timeout, func() (bool, error) {
		job, err := client.Batch().Jobs(tsuruNamespace).Get(jobName)
		if err != nil {
			if waitDelete && k8sErrors.IsNotFound(err) {
				return true, nil
			}
			return false, errors.WithStack(err)
		}
		if !waitDelete {
			if job.Status.Failed > 0 {
				return false, errors.Errorf("job %s failed: %#v", jobName, job.Status.Conditions)
			}
			if job.Status.Succeeded == 1 {
				return true, nil
			}
		}
		return false, nil
	})
}

func waitForDeploymentDelete(client kubernetes.Interface, depName string, timeout time.Duration) error {
	return waitFor(timeout, func() (bool, error) {
		_, err := client.Extensions().Deployments(tsuruNamespace).Get(depName)
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				return true, nil
			}
			return false, errors.WithStack(err)
		}
		return false, nil
	})
}

func cleanupPods(client kubernetes.Interface, opts v1.ListOptions) error {
	pods, err := client.Core().Pods(tsuruNamespace).List(opts)
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		err = client.Core().Pods(tsuruNamespace).Delete(pod.Name, &v1.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func cleanupReplicas(client kubernetes.Interface, opts v1.ListOptions) error {
	replicas, err := client.Extensions().ReplicaSets(tsuruNamespace).List(opts)
	if err != nil {
		return err
	}
	falseVar := false
	for _, replica := range replicas.Items {
		err = client.Extensions().ReplicaSets(tsuruNamespace).Delete(replica.Name, &v1.DeleteOptions{
			OrphanDependents: &falseVar,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func cleanupJob(client kubernetes.Interface, jobName string) error {
	falseVar := false
	err := client.Batch().Jobs(tsuruNamespace).Delete(jobName, &v1.DeleteOptions{
		OrphanDependents: &falseVar,
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	err = waitForJob(client, jobName, defaultBuildJobTimeout, true)
	if err != nil {
		return err
	}
	return cleanupPods(client, v1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
}
