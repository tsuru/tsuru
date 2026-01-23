// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	k8sLabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"

	eventTypes "github.com/tsuru/tsuru/types/event"
	jobTypes "github.com/tsuru/tsuru/types/job"
)

const (
	promNamespace = "tsuru"
	promSubsystem = "job"
	expireTTL     = time.Hour * 24 // 1 day

	jobSecretPrefix = "tsuru-job-"
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
	disableSecrets := client.disableSecrets(job.Pool)

	jSpec := job.Spec

	secretName := jobSecretPrefix + job.Name

	requirements, err := resourceRequirements(&job.Plan, job.Pool, client, requirementsFactors{})
	if err != nil {
		return batchv1.JobSpec{}, err
	}

	envs := []apiv1.EnvVar{}

	for _, env := range jSpec.Envs {
		if disableSecrets || env.Public {
			envs = append(envs, apiv1.EnvVar{
				Name:  env.Name,
				Value: strings.ReplaceAll(env.Value, "$", "$$"),
			})
		} else {
			envs = append(envs, apiv1.EnvVar{
				Name: env.Name,
				ValueFrom: &apiv1.EnvVarSource{
					SecretKeyRef: &apiv1.SecretKeySelector{
						Key: env.Name,
						LocalObjectReference: apiv1.LocalObjectReference{
							Name: secretName,
						},
					},
				},
			})
		}

	}

	for _, env := range jSpec.ServiceEnvs {
		if disableSecrets || env.Public {
			envs = append(envs, apiv1.EnvVar{
				Name:  env.Name,
				Value: strings.ReplaceAll(env.Value, "$", "$$"),
			})
		} else {
			envs = append(envs, apiv1.EnvVar{
				Name: env.Name,
				ValueFrom: &apiv1.EnvVarSource{
					SecretKeyRef: &apiv1.SecretKeySelector{
						Key: env.Name,
						LocalObjectReference: apiv1.LocalObjectReference{
							Name: secretName,
						},
					},
				},
			})
		}
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

func generateJobNameWithScheduleHash(job *jobTypes.Job) string {
	h := sha256.New()

	h.Write([]byte(job.Spec.Schedule))
	hashBytes := h.Sum(nil)
	hashString := hex.EncodeToString(hashBytes)

	scheduleHash := hashString[:8]

	// max name size will be up to 40 from name + "-" + 8 from hash
	return fmt.Sprintf("%s-%s", job.Name, scheduleHash)
}

func getCronJobWithFallback(ctx context.Context, client *ClusterClient, job *jobTypes.Job, namespace string) (*batchv1.CronJob, error) {
	newJobName := generateJobNameWithScheduleHash(job)
	cron, err := client.BatchV1().CronJobs(namespace).Get(ctx, newJobName, metav1.GetOptions{})
	if err == nil {
		return cron, nil
	}
	if !k8sErrors.IsNotFound(err) {
		return nil, errors.WithStack(err)
	}

	// Try old naming scheme
	cron, err = client.BatchV1().CronJobs(namespace).Get(ctx, job.Name, metav1.GetOptions{})
	if err == nil {
		return cron, nil
	}
	if !k8sErrors.IsNotFound(err) {
		return nil, errors.WithStack(err)
	}

	// If no direct match, find any cronjob
	allCronJobs, err := findAllCronJobsForJob(ctx, client, job.Name, namespace)
	if err != nil {
		return nil, err
	}
	if len(allCronJobs) > 0 {
		// If multiple cronjobs exist, return the most recent
		// This is a safeguard in-case a previous deletion didn't complete yet, since we do it background
		mostRecent := &allCronJobs[0]
		for i := 1; i < len(allCronJobs); i++ {
			if allCronJobs[i].CreationTimestamp.After(mostRecent.CreationTimestamp.Time) {
				mostRecent = &allCronJobs[i]
			}
		}
		return mostRecent, nil
	}

	return nil, k8sErrors.NewNotFound(batchv1.Resource("cronjob"), job.Name)
}

func findAllCronJobsForJob(ctx context.Context, client *ClusterClient, jobName, namespace string) ([]batchv1.CronJob, error) {
	listOptions := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("tsuru.io/job-name=%s", jobName),
	}
	cronJobs, err := client.BatchV1().CronJobs(namespace).List(ctx, listOptions)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return cronJobs.Items, nil
}

func ensureCronjob(ctx context.Context, client *ClusterClient, job *jobTypes.Job) error {
	labels, annotations := buildMetadata(ctx, job)
	jobSpec, err := buildJobSpec(job, client, labels, annotations)
	if err != nil {
		return err
	}
	namespace := client.PoolNamespace(job.Pool)

	existingCronjob, err := getCronJobWithFallback(ctx, client, job, namespace)
	if k8sErrors.IsNotFound(err) {
		existingCronjob = nil
	} else if err != nil {
		return errors.WithStack(err)
	}

	if existingCronjob != nil && existingCronjob.Spec.Schedule != job.Spec.Schedule {
		var cronJobsToDelete []batchv1.CronJob

		if existingCronjob.Name == job.Name {
			cronJobsToDelete = []batchv1.CronJob{*existingCronjob}
		} else {
			allCronJobs, err := findAllCronJobsForJob(ctx, client, job.Name, namespace)
			if err != nil {
				return errors.WithStack(err)
			}
			cronJobsToDelete = allCronJobs
		}

		propagationPolicy := metav1.DeletePropagationBackground

		for _, cronJob := range cronJobsToDelete {
			err = client.BatchV1().CronJobs(namespace).Delete(ctx, cronJob.Name, metav1.DeleteOptions{
				// NOTE: 1s is used so we can avoid possible race conditions on resources that the cron job can be using under the hood
				// Similar to: https://github.com/kubernetes/kubernetes/issues/120671
				GracePeriodSeconds: ptr.To[int64](1),
				PropagationPolicy:  &propagationPolicy,
			})

			if err != nil && !k8sErrors.IsNotFound(err) {
				return errors.WithStack(err)
			}
		}

		existingCronjob = nil
	}

	concurrencyPolicy := ""
	if job.Spec.ConcurrencyPolicy != nil {
		concurrencyPolicy = *job.Spec.ConcurrencyPolicy
	}

	var cronjobName string
	if existingCronjob != nil {
		cronjobName = existingCronjob.Name
	} else {
		cronjobName = generateJobNameWithScheduleHash(job)
	}

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cronjobName,
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

func ensureSecretForJob(ctx context.Context, client *ClusterClient, job *jobTypes.Job) (*apiv1.Secret, error) {
	labels := provision.SecretLabels(provision.SecretLabelsOpts{
		Job:    job,
		Prefix: tsuruLabelPrefix,
	}).ToLabels()
	secretName := jobSecretPrefix + job.Name
	namespace := client.PoolNamespace(job.Pool)

	existingCronjob, err := getCronJobWithFallback(ctx, client, job, namespace)
	if k8sErrors.IsNotFound(err) {
		existingCronjob = nil
	} else if err != nil {
		return nil, errors.WithStack(err)
	}

	if err != nil && !k8sErrors.IsNotFound(err) {
		return nil, errors.WithStack(err)
	}

	data := map[string][]byte{}

	for _, env := range job.Spec.Envs {
		if env.Public {
			continue
		}

		data[env.Name] = []byte(env.Value)
	}

	for _, env := range job.Spec.ServiceEnvs {
		if env.Public {
			continue
		}
		data[env.Name] = []byte(env.Value)
	}

	oldSecret, err := client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	oldSecretNotFound := k8sErrors.IsNotFound(err)
	if !oldSecretNotFound && err != nil {
		return nil, errors.WithStack(err)
	}

	secret := apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      secretName,
			Labels:    labels,
		},
		Type: apiv1.SecretTypeOpaque,
		Data: data,
	}

	if existingCronjob != nil {
		secret.OwnerReferences = []metav1.OwnerReference{
			*metav1.NewControllerRef(existingCronjob, batchv1.SchemeGroupVersion.WithKind("CronJob")),
		}
	}

	if oldSecretNotFound {
		newSecret, err := client.CoreV1().Secrets(namespace).Create(ctx, &secret, metav1.CreateOptions{})
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return newSecret, nil
	}

	if secretUnchanged(oldSecret, &secret) {
		return oldSecret, nil
	}

	secret.ResourceVersion = oldSecret.ResourceVersion
	secret.Finalizers = oldSecret.Finalizers

	return client.CoreV1().Secrets(namespace).Update(ctx, &secret, metav1.UpdateOptions{})
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

func (p *kubernetesProvisioner) EnsureJob(ctx context.Context, job *jobTypes.Job) error {
	client, err := clusterForPool(ctx, job.Pool)
	if err != nil {
		return err
	}
	err = ensureServiceAccountForJob(ctx, client, *job)
	if err != nil {
		return err
	}

	err = ensureCronjob(ctx, client, job)
	if err != nil {
		return err
	}

	_, err = ensureSecretForJob(ctx, client, job)
	if err != nil {
		return err
	}

	return nil
}

func (p *kubernetesProvisioner) TriggerCron(ctx context.Context, job *jobTypes.Job, pool string) error {
	client, err := clusterForPool(ctx, pool)
	if err != nil {
		return err
	}
	namespace := client.PoolNamespace(pool)

	cron, err := getCronJobWithFallback(ctx, client, job, namespace)
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
	cronChild.Name = getManualJobName(job.Name)
	if cronChild.Annotations == nil {
		cronChild.Annotations = map[string]string{"cronjob.kubernetes.io/instantiate": "manual"}
	} else {
		cronChild.Annotations["cronjob.kubernetes.io/instantiate"] = "manual"
	}
	_, err = client.BatchV1().Jobs(cron.Namespace).Create(ctx, &cronChild, metav1.CreateOptions{})
	if err != nil && k8sErrors.IsAlreadyExists(err) {
		return errors.Errorf("manual job %q already exists (cronjobs can only be triggered once per minute)", cronChild.Name)
	}
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
		LabelSelector: k8sLabels.Set(labelSelector.MatchLabels).String(),
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
	if err = client.CoreV1().ServiceAccounts(namespace).Delete(ctx, serviceAccountNameForJob(*job), metav1.DeleteOptions{}); err != nil && !k8sErrors.IsNotFound(err) {
		return err
	}

	secretName := jobSecretPrefix + job.Name
	if err = client.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{}); err != nil && !k8sErrors.IsNotFound(err) {
		return err
	}

	jobName := generateJobNameWithScheduleHash(job)
	err = client.BatchV1().CronJobs(namespace).Delete(ctx, jobName, metav1.DeleteOptions{})
	if err != nil && k8sErrors.IsNotFound(err) {
		// Fallback to old naming scheme
		err = client.BatchV1().CronJobs(namespace).Delete(ctx, job.Name, metav1.DeleteOptions{})
	}
	if err != nil && !k8sErrors.IsNotFound(err) {
		return err
	}

	return nil
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
	if jobNameLabel, ok := k8sJob.Labels[tsuruLabelJobName]; !ok || jobNameLabel != job.Name {
		return &provision.UnitNotFoundError{ID: unit}
	}

	pods, err := p.getPodsForJob(ctx, client, k8sJob)
	if err != nil {
		return errors.WithStack(err)
	}

	deleteOptions := metav1.DeleteOptions{}
	if force {
		deleteOptions = metav1.DeleteOptions{
			GracePeriodSeconds: ptr.To[int64](1),
		}
	}

	err = client.BatchV1().Jobs(namespace).Delete(ctx, unit, deleteOptions)
	if err != nil {
		return errors.WithStack(err)
	}

	if len(pods) > 0 {
		err = removePodsForJob(ctx, client, pods, deleteOptions)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func removePodsForJob(ctx context.Context, client *ClusterClient, podList []apiv1.Pod, deleteOptions metav1.DeleteOptions) error {
	for _, pod := range podList {
		if err := client.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, deleteOptions); err != nil {
			if k8sErrors.IsNotFound(err) {
				continue
			}
			return errors.WithStack(err)
		}
	}

	return nil
}

func (p *kubernetesProvisioner) getPodsForJob(ctx context.Context, client *ClusterClient, job *batchv1.Job) ([]apiv1.Pod, error) {
	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"job-name": job.Name}}
	listOptions := metav1.ListOptions{
		LabelSelector: k8sLabels.Set(labelSelector.MatchLabels).String(),
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
		var statusReason string
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
			statusReason = findJobFailedReason(&k8sJob)
		case k8sJob.Status.Succeeded > 0:
			status = provTypes.UnitStatusSucceeded
		default:
			status = provTypes.UnitStatusStarted
		}

		createdAt := k8sJob.CreationTimestamp.Time.In(time.UTC)
		units = append(units, provTypes.Unit{
			ID:           k8sJob.Name,
			Name:         k8sJob.Name,
			Status:       status,
			StatusReason: statusReason,
			Restarts:     &restarts,
			CreatedAt:    &createdAt,
		})
	}
	return units, nil
}

func findJobFailedReason(job *batchv1.Job) string {
	if job.Status.Conditions != nil {
		for _, condition := range job.Status.Conditions {
			if condition.Type == batchv1.JobFailed && condition.Status == apiv1.ConditionTrue {
				return condition.Reason
			}
		}
	}
	return ""
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
	var kind *permTypes.PermissionScheme
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
