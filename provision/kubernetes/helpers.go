// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/set"
	"k8s.io/client-go/kubernetes"
	k8sErrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/labels"
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

type labelSet struct {
	labels      map[string]string
	annotations map[string]string
}

func withPrefix(m map[string]string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		if !strings.HasPrefix(k, tsuruLabelPrefix) {
			k = tsuruLabelPrefix + k
		}
		result[k] = v
	}
	return result
}

func subMap(m map[string]string, keys ...string) map[string]string {
	result := make(map[string]string, len(keys))
	s := set.FromValues(keys...)
	for k, v := range m {
		if s.Includes(k) {
			result[k] = v
		}
	}
	return result
}

func (s *labelSet) ToLabels() map[string]string {
	return withPrefix(s.labels)
}

func (s *labelSet) ToAnnotations() map[string]string {
	return withPrefix(s.annotations)
}

func (s *labelSet) ToSelector() map[string]string {
	return withPrefix(subMap(s.labels, "app-name", "app-process", "is-build"))
}

func (s *labelSet) ToAppSelector() map[string]string {
	return withPrefix(subMap(s.labels, "app-name"))
}

func (s *labelSet) AppProcess() string {
	return s.getLabel("app-process")
}

func (s *labelSet) AppPlatform() string {
	return s.getLabel("app-platform")
}

func (s *labelSet) AppReplicas() int {
	replicas, _ := strconv.Atoi(s.getLabel("app-process-replicas"))
	return replicas
}

func (s *labelSet) Restarts() int {
	restarts, _ := strconv.Atoi(s.getLabel("restarts"))
	return restarts
}

func (s *labelSet) BuildImage() string {
	return s.getLabel("build-image")
}

func (s *labelSet) SetRestarts(count int) {
	s.addLabel("restarts", strconv.Itoa(count))
}

func (s *labelSet) addLabel(k, v string) {
	s.labels[k] = v
}

func (s *labelSet) getLabel(k string) string {
	if v, ok := s.labels[tsuruLabelPrefix+k]; ok {
		return v
	}
	if v, ok := s.labels[k]; ok {
		return v
	}
	if v, ok := s.annotations[tsuruLabelPrefix+k]; ok {
		return v
	}
	if v, ok := s.annotations[k]; ok {
		return v
	}
	return ""
}

func labelSetFromMeta(meta *v1.ObjectMeta) *labelSet {
	return &labelSet{labels: meta.Labels, annotations: meta.Annotations}
}

func podLabels(a provision.App, process, buildImg string, replicas int) (*labelSet, error) {
	routerName, err := a.GetRouterName()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	routerType, _, err := router.Type(routerName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	set := &labelSet{
		labels: map[string]string{
			"is-tsuru":             strconv.FormatBool(true),
			"is-build":             strconv.FormatBool(buildImg != ""),
			"app-name":             a.GetName(),
			"app-process":          process,
			"app-process-replicas": strconv.Itoa(replicas),
			"app-platform":         a.GetPlatform(),
			"app-pool":             a.GetPool(),
			"router-name":          routerName,
			"router-type":          routerType,
			"provisioner":          "kubernetes",
		},
		annotations: map[string]string{
			"build-image": buildImg,
		},
	}
	return set, nil
}
