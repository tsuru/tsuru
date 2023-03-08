// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"
	"time"

	"github.com/tsuru/tsuru/provision"
	jobTypes "github.com/tsuru/tsuru/types/job"
	batchv1 "k8s.io/api/batch/v1"
	apiv1beta1 "k8s.io/api/batch/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func createJobSpec(containerInfo jobTypes.ContainerInfo, labels, annotations map[string]string) batchv1.JobSpec {
	return batchv1.JobSpec{
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      labels,
				Annotations: annotations,
			},
			Spec: v1.PodSpec{
				RestartPolicy: "OnFailure",
				Containers: []v1.Container{
					{
						Name:    containerInfo.Name,
						Image:   containerInfo.Image,
						Command: containerInfo.Command,
					},
				},
			},
		},
	}
}

func createCronjob(ctx context.Context, client *ClusterClient, job provision.Job, jobSpec batchv1.JobSpec, labels, annotations map[string]string) (string, error) {
	k8sCronjob, err := client.BatchV1beta1().CronJobs(client.Namespace()).Create(ctx, &apiv1beta1.CronJob{
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
	return k8sCronjob.Name, err
}

func createJob(ctx context.Context, client *ClusterClient, job provision.Job, jobSpec batchv1.JobSpec, labels map[string]string, annotations map[string]string) (string, error) {
	k8sJob, err := client.BatchV1().Jobs(client.Namespace()).Create(ctx, &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        job.GetName(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: jobSpec,
	}, metav1.CreateOptions{})
	return k8sJob.Name, err
}

func (p *kubernetesProvisioner) CreateJob(ctx context.Context, j provision.Job) (string, error) {
	client, err := clusterForPool(ctx, j.GetPool())
	if err != nil {
		return "", err
	}
	jobLabels := provision.JobLabels(ctx, j).ToLabels()
	jobAnnotations := map[string]string{}
	for _, a := range j.GetMetadata().Annotations {
		jobAnnotations[a.Name] = a.Value
	}
	jobSpec := createJobSpec(j.GetContainerInfo(), jobLabels, jobAnnotations)
	if j.IsCron() {
		return createCronjob(ctx, client, j, jobSpec, jobLabels, jobAnnotations)
	}
	return createJob(ctx, client, j, jobSpec, jobLabels, jobAnnotations)
}

func (p *kubernetesProvisioner) TriggerCron(ctx context.Context, j provision.Job) error {
	client, err := clusterForPool(ctx, j.GetPool())
	if err != nil {
		return err
	}
	cron, err := client.BatchV1beta1().CronJobs(client.Namespace()).Get(ctx, j.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}
	cronChild := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   cron.Namespace,
			Labels:      cron.Labels,
			Annotations: cron.Annotations,
		},
		Spec: cron.Spec.JobTemplate.Spec,
	}
	cronChild.OwnerReferences = []metav1.OwnerReference{
		{
			Name:       cron.Name,
			Kind:       "CronJob",
			UID:        cron.UID,
			APIVersion: "batch/v1",
		},
	}
	cronChild.Name = fmt.Sprintf("%s-manual-trigger", cron.Name)
	if cronChild.Annotations == nil {
		cronChild.Annotations = map[string]string{"cronjob.kubernetes.io/instantiate": "manual"}
	} else {
		cronChild.Annotations["cronjob.kubernetes.io/instantiate"] = "manual"
	}
	_, err = client.BatchV1().Jobs(cron.Namespace).Get(ctx, cronChild.Name, metav1.GetOptions{})
	if err == nil {
		if err = client.BatchV1().Jobs(cron.Namespace).Delete(ctx, cronChild.Name, metav1.DeleteOptions{}); err != nil {
			return err
		}
	} else if !k8sErrors.IsNotFound(err) {
		return err
	}
	_, err = client.BatchV1().Jobs(cron.Namespace).Create(ctx, &cronChild, metav1.CreateOptions{})
	return err
}

func (p *kubernetesProvisioner) UpdateJob(ctx context.Context, j provision.Job) error {
	client, err := clusterForPool(ctx, j.GetPool())
	if err != nil {
		return err
	}
	jobLabels := provision.JobLabels(ctx, j).ToLabels()
	jobAnnotations := map[string]string{}
	for _, a := range j.GetMetadata().Annotations {
		jobAnnotations[a.Name] = a.Value
	}
	jobSpec := createJobSpec(j.GetContainerInfo(), jobLabels, jobAnnotations)
	_, err = client.BatchV1beta1().CronJobs(client.Namespace()).Update(ctx, &apiv1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        j.GetName(),
			Labels:      jobLabels,
			Annotations: jobAnnotations,
		},
		Spec: apiv1beta1.CronJobSpec{
			Schedule: j.GetSchedule(),
			JobTemplate: apiv1beta1.JobTemplateSpec{
				Spec: jobSpec,
			},
		},
	}, metav1.UpdateOptions{})
	return err
}

// JobUnits returns information about units related to a specific Job or CronJob
func (p *kubernetesProvisioner) JobUnits(ctx context.Context, j provision.Job) ([]provision.Unit, error) {
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
	podList := []apiv1.Pod{}
	for _, j := range jobs {
		labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"tsuru.io/job-name": j.GetName(), "tsuru.io/job-team": j.GetTeamOwner()}}
		listOptions := metav1.ListOptions{
			LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
		}
		pods, err := client.CoreV1().Pods(client.Namespace()).List(ctx, listOptions)
		if err != nil {
			return podList, err
		}
		podList = append(podList, pods.Items...)
	}
	return podList, nil
}

func (p *kubernetesProvisioner) podsToJobUnits(ctx context.Context, client *ClusterClient, pods []apiv1.Pod, job provision.Job) ([]provision.Unit, error) {
	if len(pods) == 0 {
		return nil, nil
	}
	var units []provision.Unit
	for _, pod := range pods {
		var status provision.Status
		if pod.Status.Phase == apiv1.PodRunning {
			status, _ = extractStatusAndReasonFromContainerStatuses(pod.Status.ContainerStatuses)
		} else {
			status = stateMap[pod.Status.Phase]
		}

		createdAt := pod.CreationTimestamp.Time.In(time.UTC)
		units = append(units, provision.Unit{
			ID:        pod.Name,
			Name:      pod.Name,
			IP:        pod.Status.HostIP,
			Status:    status,
			Restarts:  containersRestarts(pod.Status.ContainerStatuses),
			CreatedAt: &createdAt,
		})
	}
	return units, nil
}
