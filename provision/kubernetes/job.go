// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/adhocore/gronx"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	permTypes "github.com/tsuru/tsuru/types/permission"
	provTypes "github.com/tsuru/tsuru/types/provision"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	eventTypes "github.com/tsuru/tsuru/types/event"
	jobTypes "github.com/tsuru/tsuru/types/job"
)

const (
	promNamespace = "tsuru"
	promSubsystem = "job"
	expireTTL     = time.Hour * 24 // 1 day
)

var (
	jobCompleted = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "completed_total",
		Help:      "The total number of completed jobs",
	}, []string{"job_name"})

	jobFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "failed_total",
		Help:      "The total number of failed jobs",
	}, []string{"job_name", "reason"})

	jobStarted = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "started_total",
		Help:      "The total number of started jobs",
	}, []string{"job_name"})
)

func buildJobSpec(job *jobTypes.Job, client *ClusterClient, labels, annotations map[string]string) (batchv1.JobSpec, error) {
	jSpec := job.Spec

	requirements, err := resourceRequirements(&job.Plan, job.Pool, client, requirementsFactors{})
	if err != nil {
		return batchv1.JobSpec{}, err
	}

	envs := []apiv1.EnvVar{}

	for _, env := range jSpec.Envs {
		envs = append(envs, apiv1.EnvVar{
			Name:  env.Name,
			Value: strings.ReplaceAll(env.Value, "$", "$$"),
		})
	}

	for _, env := range jSpec.ServiceEnvs {
		envs = append(envs, apiv1.EnvVar{
			Name:  env.Name,
			Value: strings.ReplaceAll(env.Value, "$", "$$"),
		})
	}

	imageURL := jSpec.Container.InternalRegistryImage
	if imageURL == "" {
		imageURL = jSpec.Container.OriginalImageSrc
	}

	return batchv1.JobSpec{
		Parallelism:             jSpec.Parallelism,
		BackoffLimit:            jSpec.BackoffLimit,
		Completions:             jSpec.Completions,
		ActiveDeadlineSeconds:   buildActiveDeadline(jSpec.ActiveDeadlineSeconds),
		TTLSecondsAfterFinished: func() *int32 { ttlSecondsAfterFinished := int32(86400); return &ttlSecondsAfterFinished }(), // hardcoded to a day, since we keep logs stored elsewhere on the cloud
		Template: apiv1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      labels,
				Annotations: annotations,
			},
			Spec: apiv1.PodSpec{
				RestartPolicy: "OnFailure",
				Containers: []apiv1.Container{
					{
						Name:      "job",
						Image:     imageURL,
						Command:   jSpec.Container.Command,
						Resources: requirements,
						Env:       envs,
					},
				},
				ServiceAccountName: serviceAccountNameForJob(*job),
			},
		},
	}, nil
}

func ensureCronjob(ctx context.Context, client *ClusterClient, job *jobTypes.Job) error {
	labels, annotations := buildMetadata(ctx, job)
	jobSpec, err := buildJobSpec(job, client, labels, annotations)
	if err != nil {
		return err
	}

	namespace := client.PoolNamespace(job.Pool)

	existingCronjob, err := client.BatchV1().CronJobs(namespace).Get(ctx, job.Name, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		existingCronjob = nil
	} else if err != nil {
		return errors.WithStack(err)
	}

	// when the schedule suffer changes some cronjobs may suffer a unexpected execution
	// for these reason we decided to recreate the entire cronjob to avoid this
	if existingCronjob != nil && existingCronjob.Spec.Schedule != job.Spec.Schedule {

		wait, waitErr := waitToJobDeletion(ctx, client, existingCronjob)
		if waitErr != nil {
			return errors.WithStack(waitErr)
		}

		propagationPolicy := metav1.DeletePropagationForeground
		err = client.BatchV1().CronJobs(namespace).Delete(ctx, existingCronjob.Name, metav1.DeleteOptions{
			GracePeriodSeconds: ptr.To[int64](0),
			PropagationPolicy:  &propagationPolicy,
		})

		if err != nil {
			return errors.WithStack(err)
		}
		existingCronjob = nil

		waitErr = wait()
		if waitErr != nil {
			return errors.WithStack(waitErr)
		}
	}

	concurrencyPolicy := ""
	if job.Spec.ConcurrencyPolicy != nil {
		concurrencyPolicy = *job.Spec.ConcurrencyPolicy
	}

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        job.Name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: job.Spec.Schedule,
			Suspend:  &job.Spec.Manual,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: jobSpec,
			},
			ConcurrencyPolicy: batchv1.ConcurrencyPolicy(concurrencyPolicy),
		},
	}

	if existingCronjob == nil {
		_, err = client.BatchV1().CronJobs(namespace).Create(ctx, cronjob, metav1.CreateOptions{})

		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	cronjob.ResourceVersion = existingCronjob.ResourceVersion
	cronjob.Finalizers = existingCronjob.Finalizers

	_, err = client.BatchV1().CronJobs(namespace).Update(ctx, cronjob, metav1.UpdateOptions{})

	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func buildMetadata(ctx context.Context, job *jobTypes.Job) (map[string]string, map[string]string) {
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

type deleteWaiterFunc func() error

func waitToJobDeletion(ctx context.Context, client kubernetes.Interface, existingCronjob *batchv1.CronJob) (deleteWaiterFunc, error) {
	deleted := make(chan struct{}, 1)

	watchInterface, err := client.BatchV1().CronJobs(existingCronjob.ObjectMeta.Namespace).Watch(ctx, metav1.ListOptions{
		Watch:           true,
		FieldSelector:   "metadata.name=" + existingCronjob.ObjectMeta.Name,
		ResourceVersion: existingCronjob.ObjectMeta.ResourceVersion,
	})

	if err != nil {
		return nil, err
	}

	go func() {
		for {
			event := <-watchInterface.ResultChan()

			if event.Type == watch.Deleted {
				close(deleted)
				break
			}
		}
	}()

	return func() error {
		select {
		case <-deleted:
			return nil
		case <-time.After(time.Second * 30):
			return errors.New("timeout waiting delete")
		}
	}, nil
}

func (p *kubernetesProvisioner) EnsureJob(ctx context.Context, job *jobTypes.Job) error {
	client, err := clusterForPool(ctx, job.Pool)
	if err != nil {
		return err
	}
	if err = ensureServiceAccountForJob(ctx, client, *job); err != nil {
		return err
	}

	return ensureCronjob(ctx, client, job)
}

func (p *kubernetesProvisioner) TriggerCron(ctx context.Context, name, pool string) error {
	client, err := clusterForPool(ctx, pool)
	if err != nil {
		return err
	}
	namespace := client.PoolNamespace(pool)
	cron, err := client.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
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
	cronChild.Name = getManualJobName(cron.Name)
	if cronChild.Annotations == nil {
		cronChild.Annotations = map[string]string{"cronjob.kubernetes.io/instantiate": "manual"}
	} else {
		cronChild.Annotations["cronjob.kubernetes.io/instantiate"] = "manual"
	}
	_, err = client.BatchV1().Jobs(cron.Namespace).Create(ctx, &cronChild, metav1.CreateOptions{})
	return err
}

func getManualJobName(job string) string {
	scheduledTime := time.Now()
	return fmt.Sprintf("%s-manual-job-%d", job, scheduledTime.Unix()/60)
}

// JobUnits returns information about units related to a specific Job or CronJob
func (p *kubernetesProvisioner) JobUnits(ctx context.Context, job *jobTypes.Job) ([]provTypes.Unit, error) {
	client, err := clusterForPool(ctx, job.Pool)
	if err != nil {
		return nil, err
	}
	jobLabels := provision.JobLabels(ctx, job).ToLabels()
	labelSelector := metav1.LabelSelector{MatchLabels: jobLabels}
	listOptions := metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
	}
	k8sJobs, err := client.BatchV1().Jobs(client.PoolNamespace(job.Pool)).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	return p.jobsToJobUnits(ctx, client, k8sJobs.Items)
}

func (p *kubernetesProvisioner) DestroyJob(ctx context.Context, job *jobTypes.Job) error {
	client, err := clusterForPool(ctx, job.Pool)
	if err != nil {
		return err
	}
	namespace := client.PoolNamespace(job.Pool)
	if err := client.CoreV1().ServiceAccounts(namespace).Delete(ctx, serviceAccountNameForJob(*job), metav1.DeleteOptions{}); err != nil && !k8sErrors.IsNotFound(err) {
		return err
	}
	return client.BatchV1().CronJobs(namespace).Delete(ctx, job.Name, metav1.DeleteOptions{})
}

func (p *kubernetesProvisioner) KillJobUnit(ctx context.Context, job *jobTypes.Job, unit string, force bool) error {
	client, err := clusterForPool(ctx, job.Pool)
	if err != nil {
		return err
	}
	namespace := client.PoolNamespace(job.Pool)
	k8sJob, err := client.BatchV1().Jobs(namespace).Get(ctx, unit, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return &provision.UnitNotFoundError{ID: unit}
		}
		return errors.WithStack(err)
	}
	if jobName, ok := k8sJob.Labels[tsuruLabelJobName]; !ok || jobName != job.Name {
		return &provision.UnitNotFoundError{ID: unit}
	}
	if force {
		return client.BatchV1().Jobs(namespace).Delete(ctx, unit, metav1.DeleteOptions{GracePeriodSeconds: func() *int64 { i := int64(0); return &i }()})
	}
	return client.BatchV1().Jobs(namespace).Delete(ctx, unit, metav1.DeleteOptions{})
}

func (p *kubernetesProvisioner) getPodsForJob(ctx context.Context, client *ClusterClient, job *batchv1.Job) ([]apiv1.Pod, error) {
	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"job-name": job.Name}}
	listOptions := metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
	}
	pods, err := client.CoreV1().Pods(job.Namespace).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func (p *kubernetesProvisioner) jobsToJobUnits(ctx context.Context, client *ClusterClient, k8sJobs []batchv1.Job) ([]provTypes.Unit, error) {
	if len(k8sJobs) == 0 {
		return nil, nil
	}
	var units []provTypes.Unit
	for _, k8sJob := range k8sJobs {
		var status provTypes.UnitStatus
		var restarts int32
		pods, err := p.getPodsForJob(ctx, client, &k8sJob)
		if err != nil {
			return nil, err
		}
		for _, pod := range pods {
			restarts += *containersRestarts(pod.Status.ContainerStatuses)
		}
		switch {
		case k8sJob.Status.Failed > 0:
			status = provTypes.UnitStatusError
		case k8sJob.Status.Succeeded > 0:
			status = provTypes.UnitStatusSucceeded
		default:
			status = provTypes.UnitStatusStarted
		}

		createdAt := k8sJob.CreationTimestamp.Time.In(time.UTC)
		units = append(units, provTypes.Unit{
			ID:        k8sJob.Name,
			Name:      k8sJob.Name,
			Status:    status,
			Restarts:  &restarts,
			CreatedAt: &createdAt,
		})
	}
	return units, nil
}

func incrementJobMetrics(job *batchv1.Job, evt *apiv1.Event, wg *sync.WaitGroup) {
	defer wg.Done()
	jobName := job.Labels[tsuruLabelJobName]
	switch evt.Reason {
	case "Completed":
		jobCompleted.WithLabelValues(jobName).Inc()
	case "BackoffLimitExceeded":
		jobFailed.WithLabelValues(jobName, evt.Message).Inc()
	case "SuccessfulCreate":
		jobStarted.WithLabelValues(jobName).Inc()
	default:
		return
	}
}

func generateExpireEventTime(clusterClient *ClusterClient, job *batchv1.Job) time.Time {
	now := time.Now().UTC()
	defaultExpire := now.Add(expireTTL)
	cj, err := clusterClient.BatchV1().CronJobs(job.Namespace).Get(context.Background(), job.Name, metav1.GetOptions{})
	if err != nil {
		return defaultExpire
	}
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		return defaultExpire
	}
	nextTime, err := gronx.NextTickAfter(cj.Spec.Schedule, now, false)
	if err != nil {
		return defaultExpire
	}
	if nextTime.Sub(now) < time.Hour {
		return now.Add(time.Hour)
	}
	return defaultExpire
}

func createJobEvent(clusterClient *ClusterClient, job *batchv1.Job, evt *apiv1.Event, wg *sync.WaitGroup) {
	ctx := context.Background()
	defer wg.Done()
	var evtErr error
	var kind *permission.PermissionScheme
	switch evt.Reason {
	case "Completed":
		kind = permission.PermJobRun
	case "BackoffLimitExceeded":
		kind = permission.PermJobRun
		evtErr = errors.New(fmt.Sprintf("job failed: %s", evt.Message))
	default:
		return
	}
	realJobOwner := job.Name
	for _, owner := range job.OwnerReferences {
		if owner.Kind == "CronJob" {
			realJobOwner = owner.Name
		}
	}

	expire := generateExpireEventTime(clusterClient, job)
	opts := event.Opts{
		Kind:       kind,
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeJob, Value: realJobOwner},
		Allowed:    event.Allowed(permission.PermJobReadEvents, permission.Context(permTypes.CtxJob, realJobOwner)),
		RawOwner:   eventTypes.Owner{Type: eventTypes.OwnerTypeInternal},
		Cancelable: false,
		ExpireAt:   &expire,
	}
	e, err := event.New(ctx, &opts)
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
	e.DoneCustomData(ctx, evtErr, customData)
}

func ensureServiceAccountForJob(ctx context.Context, client *ClusterClient, job jobTypes.Job) error {
	labels := provision.ServiceAccountLabels(provision.ServiceAccountLabelsOpts{
		Job:    &job,
		Prefix: tsuruLabelPrefix,
	})
	ns := client.PoolNamespace(job.Pool)
	return ensureServiceAccount(ctx, client, serviceAccountNameForJob(job), labels, ns, &job.Metadata)
}

func buildActiveDeadline(activeDeadlineSeconds *int64) *int64 {
	defaultActiveDeadline := int64(60 * 60)
	if activeDeadlineSeconds == nil || *activeDeadlineSeconds == int64(0) {
		return &defaultActiveDeadline
	}
	return activeDeadlineSeconds
}
