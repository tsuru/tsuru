// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	permTypes "github.com/tsuru/tsuru/types/permission"
	batchv1 "k8s.io/api/batch/v1"
	apiv1beta1 "k8s.io/api/batch/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	jobTypes "github.com/tsuru/tsuru/types/job"
)

func createJobSpec(job *jobTypes.Job, client *ClusterClient, labels, annotations map[string]string) (batchv1.JobSpec, error) {
	jSpec := job.Spec
	requirements, err := resourceRequirements(job, client, requirementsFactors{})
	if err != nil {
		return batchv1.JobSpec{}, err
	}

	envs := []v1.EnvVar{}

	for _, env := range jSpec.Envs {
		envs = append(envs, v1.EnvVar{
			Name:  env.Name,
			Value: strings.ReplaceAll(env.Value, "$", "$$"),
		})
	}

	for _, env := range jSpec.ServiceEnvs {
		envs = append(envs, v1.EnvVar{
			Name:  env.Name,
			Value: strings.ReplaceAll(env.Value, "$", "$$"),
		})
	}

	return batchv1.JobSpec{
		Parallelism:           jSpec.Parallelism,
		BackoffLimit:          jSpec.BackoffLimit,
		Completions:           jSpec.Completions,
		ActiveDeadlineSeconds: jSpec.ActiveDeadlineSeconds,
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      labels,
				Annotations: annotations,
			},
			Spec: v1.PodSpec{
				RestartPolicy: "OnFailure",
				Containers: []v1.Container{
					{
						Name:      "job",
						Image:     jSpec.Container.Image,
						Command:   jSpec.Container.Command,
						Resources: requirements,
						Env:       envs,
					},
				},
			},
		},
	}, nil
}

func createCronjob(ctx context.Context, client *ClusterClient, job *jobTypes.Job, jobSpec batchv1.JobSpec, labels, annotations map[string]string) (string, error) {
	namespace := client.PoolNamespace(job.Pool)
	k8sCronjob, err := client.BatchV1beta1().CronJobs(namespace).Create(ctx, &apiv1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        job.Name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: apiv1beta1.CronJobSpec{
			Schedule: job.Spec.Schedule,
			Suspend:  &job.Spec.Suspended,
			JobTemplate: apiv1beta1.JobTemplateSpec{
				Spec: jobSpec,
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return k8sCronjob.Name, nil
}

func genJobMetadata(ctx context.Context, job *jobTypes.Job) (map[string]string, map[string]string) {
	jobLabels := provision.JobLabels(ctx, job).ToLabels()
	customData := job.Metadata
	for _, label := range customData.Labels {
		// don't let custom labels overwrite tsuru labels
		if _, ok := jobLabels[label.Name]; ok {
			continue
		}
		jobLabels[label.Name] = label.Value
	}
	jobAnnotations := map[string]string{}
	for _, a := range job.Metadata.Annotations {
		jobAnnotations[a.Name] = a.Value
	}
	return jobLabels, jobAnnotations
}

func (p *kubernetesProvisioner) CreateJob(ctx context.Context, job *jobTypes.Job) (string, error) {
	client, err := clusterForPool(ctx, job.Pool)
	if err != nil {
		return "", err
	}
	jobLabels, jobAnnotations := genJobMetadata(ctx, job)
	jobSpec, err := createJobSpec(job, client, jobLabels, jobAnnotations)
	if err != nil {
		return "", err
	}
	return createCronjob(ctx, client, job, jobSpec, jobLabels, jobAnnotations)
}

func (p *kubernetesProvisioner) TriggerCron(ctx context.Context, name, pool string) error {
	client, err := clusterForPool(ctx, pool)
	if err != nil {
		return err
	}
	namespace := client.PoolNamespace(pool)
	cron, err := client.BatchV1beta1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
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

func (p *kubernetesProvisioner) UpdateJob(ctx context.Context, job *jobTypes.Job) error {
	client, err := clusterForPool(ctx, job.Pool)
	if err != nil {
		return err
	}
	jobLabels, jobAnnotations := genJobMetadata(ctx, job)
	jobSpec, err := createJobSpec(job, client, jobLabels, jobAnnotations)
	if err != nil {
		return err
	}
	namespace := client.PoolNamespace(job.Pool)
	_, err = client.BatchV1beta1().CronJobs(namespace).Update(ctx, &apiv1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        job.Name,
			Namespace:   namespace,
			Labels:      jobLabels,
			Annotations: jobAnnotations,
		},
		Spec: apiv1beta1.CronJobSpec{
			Schedule: job.Spec.Schedule,
			Suspend:  &job.Spec.Suspended,
			JobTemplate: apiv1beta1.JobTemplateSpec{
				Spec: jobSpec,
			},
		},
	}, metav1.UpdateOptions{})
	return err
}

// JobUnits returns information about units related to a specific Job or CronJob
func (p *kubernetesProvisioner) JobUnits(ctx context.Context, job *jobTypes.Job) ([]provision.Unit, error) {
	client, err := clusterForPool(ctx, job.Pool)
	if err != nil {
		return nil, err
	}
	pods, err := p.podsForJobs(ctx, client, []*jobTypes.Job{job})
	if err != nil {
		return nil, err
	}
	return p.podsToJobUnits(ctx, client, pods, job)
}

func (p *kubernetesProvisioner) DestroyJob(ctx context.Context, job *jobTypes.Job) error {
	client, err := clusterForPool(ctx, job.Pool)
	if err != nil {
		return err
	}
	namespace := client.PoolNamespace(job.Pool)
	return client.BatchV1beta1().CronJobs(namespace).Delete(ctx, job.Name, metav1.DeleteOptions{})
}

func (p *kubernetesProvisioner) podsForJobs(ctx context.Context, client *ClusterClient, jobs []*jobTypes.Job) ([]apiv1.Pod, error) {
	podList := []apiv1.Pod{}
	for _, j := range jobs {
		labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{tsuruLabelJobName: j.Name}}
		listOptions := metav1.ListOptions{
			LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
		}
		namespace := client.PoolNamespace(j.Pool)

		pods, err := client.CoreV1().Pods(namespace).List(ctx, listOptions)
		if err != nil {
			return podList, err
		}
		podList = append(podList, pods.Items...)
	}
	return podList, nil
}

func (p *kubernetesProvisioner) podsToJobUnits(ctx context.Context, client *ClusterClient, pods []apiv1.Pod, job *jobTypes.Job) ([]provision.Unit, error) {
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

func createJobEvent(job *batchv1.Job, evt *apiv1.Event) {
	var evtErr error
	var kind *permission.PermissionScheme
	switch evt.Reason {
	case "Completed":
		kind = permission.PermJobRun
	case "BackoffLimitExceeded":
		kind = permission.PermJobRun
		evtErr = errors.New(fmt.Sprintf("job failed: %s", evt.Message))
	case "SuccessfulCreate":
		kind = permission.PermJobCreate
	default:
		return
	}

	realJobOwner := job.Name
	for _, owner := range job.OwnerReferences {
		if owner.Kind == "CronJob" {
			realJobOwner = owner.Name
		}
	}
	opts := event.Opts{
		Kind:       kind,
		Target:     event.Target{Type: event.TargetTypeJob, Value: realJobOwner},
		Allowed:    event.Allowed(permission.PermJobReadEvents, permission.Context(permTypes.CtxJob, realJobOwner)),
		RawOwner:   event.Owner{Type: event.OwnerTypeInternal},
		Cancelable: false,
	}
	e, err := event.New(&opts)
	if err != nil {
		return
	}
	customData := map[string]string{
		"job-name":           job.Name,
		"job-controller":     realJobOwner,
		"event-type":         evt.Type,
		"event-reason":       evt.Reason,
		"message":            evt.Message,
		"cluster-start-time": evt.CreationTimestamp.String(),
	}
	e.DoneCustomData(evtErr, customData)
}
