// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"time"

	"github.com/tsuru/tsuru/provision"
	jobTypes "github.com/tsuru/tsuru/types/job"
	batchv1 "k8s.io/api/batch/v1"
	apiv1beta1 "k8s.io/api/batch/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

func createJobSpec(containersInfo []jobTypes.ContainerInfo, labels, annotations map[string]string) batchv1.JobSpec {
	jobContainers := []v1.Container{}
	for _, ci := range containersInfo {
		jobContainers = append(jobContainers, v1.Container{
			Name:    ci.Name,
			Image:   ci.Image,
			Command: ci.Command,
		})
	}

	return batchv1.JobSpec{
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      labels,
				Annotations: annotations,
			},
			Spec: v1.PodSpec{
				RestartPolicy: "OnFailure",
				Containers:    jobContainers,
			},
		},
	}
}

func createCronjob(ctx context.Context, client *ClusterClient, job provision.Job, jobSpec batchv1.JobSpec, labels, annotations map[string]string) error {
	_, err := client.BatchV1beta1().CronJobs(client.Namespace()).Create(ctx, &apiv1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        job.GetName(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: apiv1beta1.CronJobSpec{
			Schedule: job.GetSchedule(),
			JobTemplate: apiv1beta1.JobTemplateSpec{
				Spec: jobSpec,
			},
		},
	}, metav1.CreateOptions{})
	return err
}

func createJob(ctx context.Context, client *ClusterClient, job provision.Job, jobSpec batchv1.JobSpec, labels map[string]string, annotations map[string]string) error {
	_, err := client.BatchV1().Jobs(client.Namespace()).Create(ctx, &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        job.GetName(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: jobSpec,
	}, metav1.CreateOptions{})
	return err
}

func (p *kubernetesProvisioner) CreateJob(ctx context.Context, j provision.Job) error {
	client, err := clusterForPool(ctx, j.GetPool())
	if err != nil {
		return err
	}
	jobLabels := provision.JobLabels(ctx, j).ToLabels()
	jobAnnotations := map[string]string{}
	for _, a := range j.GetMetadata().Annotations {
		jobAnnotations[a.Name] = a.Value
	}
	jobSpec := createJobSpec(j.GetContainersInfo(), jobLabels, jobAnnotations)
	if j.IsCron() {
		return createCronjob(ctx, client, j, jobSpec, jobLabels, jobAnnotations)
	}
	return createJob(ctx, client, j, jobSpec, jobLabels, jobAnnotations)
}

// JobUnits returns information about units related to a specific Job or CronJob
func (p *kubernetesProvisioner) JobUnits(ctx context.Context, j provision.Job) ([]provision.JobUnit, error) {
	client, err := clusterForPool(ctx, j.GetPool())
	if err != nil {
		return nil, err
	}
	pods, err := p.podsForJobs(ctx, client, []provision.Job{j})
	if err != nil {
		return nil, err
	}
	return p.podsToJobUnits(ctx, client, pods, j)
}

func (p *kubernetesProvisioner) DestroyJob(ctx context.Context, j provision.Job) error {
	client, err := clusterForPool(ctx, j.GetPool())
	if err != nil {
		return err
	}
	if j.IsCron() {
		return client.BatchV1beta1().CronJobs(client.Namespace()).Delete(ctx, j.GetName(), metav1.DeleteOptions{})
	}
	return client.BatchV1().Jobs(client.Namespace()).Delete(ctx, j.GetName(), metav1.DeleteOptions{})
}

func (p *kubernetesProvisioner) podsForJobs(ctx context.Context, client *ClusterClient, jobs []provision.Job) ([]apiv1.Pod, error) {
	inSelectorMap := map[string][]string{}
	for _, j := range jobs {
		l := provision.JobLabels(ctx, j)
		jobSel := l.ToJobSelector()
		for k, v := range jobSel {
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

func (p *kubernetesProvisioner) podsToJobUnits(ctx context.Context, client *ClusterClient, pods []apiv1.Pod, job provision.Job) ([]provision.JobUnit, error) {
	if len(pods) == 0 {
		return nil, nil
	}
	var units []provision.JobUnit
	for _, pod := range pods {
		l := labelSetFromMeta(&pod.ObjectMeta)

		var status provision.Status
		if pod.Status.Phase == apiv1.PodRunning {
			status = extractStatusFromContainerStatuses(pod.Status.ContainerStatuses)
		} else {
			status = stateMap[pod.Status.Phase]
		}

		createdAt := pod.CreationTimestamp.Time.In(time.UTC)
		units = append(units, provision.JobUnit{
			ID:        pod.Name,
			Name:      pod.Name,
			JobName:   l.JobName(),
			IP:        pod.Status.HostIP,
			Status:    status,
			Restarts:  containersRestarts(pod.Status.ContainerStatuses),
			CreatedAt: &createdAt,
		})
	}
	return units, nil
}
