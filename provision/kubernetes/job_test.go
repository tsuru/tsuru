// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/types/app"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	eventTypes "github.com/tsuru/tsuru/types/event"
	jobTypes "github.com/tsuru/tsuru/types/job"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

func (s *S) TestProvisionerCreateCronJob(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()
	tests := []struct {
		name      string
		scenario  func()
		namespace string
		jobName   string
		assertion func(gotCron *batchv1.CronJob)
		teardown  func()
	}{
		{
			name:      "simple create cronjob with default plan",
			jobName:   "myjob-1cec0fa1", // Hash-based name for "* * * * *"
			namespace: "default",
			teardown: func() {
				j := jobTypes.Job{
					Name: "myjob",
					Pool: "test-default",
					Spec: jobTypes.JobSpec{
						Schedule: "* * * * *",
					},
				}
				err := s.p.DestroyJob(context.TODO(), &j)
				require.NoError(s.t, err)

				_, err = s.client.BatchV1().CronJobs("default").Get(context.TODO(), "myjob-1cec0fa1", metav1.GetOptions{})
				require.True(s.t, k8sErrors.IsNotFound(err))
			},
			scenario: func() {
				cj := jobTypes.Job{
					Name:      "myjob",
					TeamOwner: s.team.Name,
					Pool:      "test-default",
					Metadata: app.Metadata{
						Labels: []app.MetadataItem{
							{
								Name:  "label1",
								Value: "value1",
							},
						},
						Annotations: []app.MetadataItem{
							{
								Name:  "annotation1",
								Value: "value2",
							},
						},
					},
					Spec: jobTypes.JobSpec{
						Schedule:              "* * * * *",
						Parallelism:           func() *int32 { r := int32(3); return &r }(),
						Completions:           func() *int32 { r := int32(1); return &r }(),
						ActiveDeadlineSeconds: func() *int64 { r := int64(60 * 60 * 24); return &r }(),
						BackoffLimit:          func() *int32 { r := int32(7); return &r }(),
						Container: jobTypes.ContainerInfo{
							OriginalImageSrc: "ubuntu:latest",
							Command:          []string{"echo", "hello world"},
						},
						Envs: []bindTypes.EnvVar{
							{
								Name:   "ENV1",
								Value:  "VAL1",
								Public: false,
							},
						},
					},
				}
				err := s.p.EnsureJob(context.TODO(), &cj)
				waitCron()
				require.NoError(s.t, err)
			},
			assertion: func(gotCron *batchv1.CronJob) {
				expectedTarget := &batchv1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob-1cec0fa1", // Hash-based name for "* * * * *"
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/component":  "job",
							"app.kubernetes.io/managed-by": "tsuru",
							"app.kubernetes.io/name":       "tsuru-job",
							"app.kubernetes.io/instance":   "myjob",
							"tsuru.io/is-tsuru":            "true",
							"tsuru.io/is-service":          "true",
							"tsuru.io/job-name":            "myjob",
							"tsuru.io/job-pool":            "test-default",
							"tsuru.io/job-team":            "admin",
							"tsuru.io/is-job":              "true",
							"tsuru.io/job-manual":          "false",
							"tsuru.io/is-build":            "false",
							"label1":                       "value1",
						},
						Annotations: map[string]string{"annotation1": "value2"},
					},
					Spec: batchv1.CronJobSpec{
						Schedule: "* * * * *",
						Suspend:  func() *bool { r := false; return &r }(),
						JobTemplate: batchv1.JobTemplateSpec{
							Spec: batchv1.JobSpec{
								TTLSecondsAfterFinished: func() *int32 { defaultTTL := int32(86400); return &defaultTTL }(),
								Parallelism:             func() *int32 { r := int32(3); return &r }(),
								Completions:             func() *int32 { r := int32(1); return &r }(),
								ActiveDeadlineSeconds:   func() *int64 { r := int64(60 * 60 * 24); return &r }(),
								BackoffLimit:            func() *int32 { r := int32(7); return &r }(),
								Template: corev1.PodTemplateSpec{
									ObjectMeta: metav1.ObjectMeta{
										Labels: map[string]string{
											"app.kubernetes.io/component":  "job",
											"app.kubernetes.io/managed-by": "tsuru",
											"app.kubernetes.io/name":       "tsuru-job",
											"app.kubernetes.io/instance":   "myjob",
											"tsuru.io/is-tsuru":            "true",
											"tsuru.io/is-service":          "true",
											"tsuru.io/job-name":            "myjob",
											"tsuru.io/job-pool":            "test-default",
											"tsuru.io/job-team":            "admin",
											"tsuru.io/is-job":              "true",
											"tsuru.io/job-manual":          "false",
											"tsuru.io/is-build":            "false",
											"label1":                       "value1",
										},
										Annotations: map[string]string{"annotation1": "value2"},
									},
									Spec: corev1.PodSpec{
										ServiceAccountName: "job-myjob",
										Containers: []corev1.Container{
											{
												Name:    "job",
												Image:   "ubuntu:latest",
												Command: []string{"echo", "hello world"},
												Env: []corev1.EnvVar{
													{
														Name: "ENV1",
														ValueFrom: &corev1.EnvVarSource{
															SecretKeyRef: &corev1.SecretKeySelector{
																Key: "ENV1",
																LocalObjectReference: corev1.LocalObjectReference{
																	Name: "tsuru-job-myjob",
																},
															},
														},
													},
												},
												Resources: corev1.ResourceRequirements{
													Limits: corev1.ResourceList{
														corev1.ResourceEphemeralStorage: resource.MustParse("100Mi"),
													},
													Requests: corev1.ResourceList{
														corev1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
													},
												},
											},
										},
										RestartPolicy: "OnFailure",
									},
								},
							},
						},
					},
				}
				require.EqualValues(s.t, *expectedTarget, *gotCron)
				jobName := "myjob" // original job name
				serviceAccountName := "job-" + jobName
				account, err := s.client.CoreV1().ServiceAccounts(expectedTarget.Namespace).Get(context.TODO(), serviceAccountName, metav1.GetOptions{})
				require.NoError(s.t, err)
				require.EqualValues(s.t, &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceAccountName,
						Namespace: expectedTarget.Namespace,
						Labels: map[string]string{
							"tsuru.io/is-tsuru": "true",
							"tsuru.io/job-name": jobName,
						},
					},
				}, account)

				secretName := "tsuru-job-" + jobName
				secret, err := s.client.CoreV1().Secrets(expectedTarget.Namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
				require.NoError(s.t, err)
				require.EqualValues(s.t, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: expectedTarget.Namespace,
						Labels: map[string]string{
							"tsuru.io/is-tsuru": "true",
							"tsuru.io/job-name": jobName,
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "batch/v1",
								Kind:               "CronJob",
								Name:               expectedTarget.Name,
								UID:                k8sTypes.UID(gotCron.UID),
								Controller:         ptr.To(true),
								BlockOwnerDeletion: ptr.To(true),
							},
						},
					},
					Data: map[string][]byte{
						"ENV1": []byte("VAL1"),
					},
					Type: corev1.SecretTypeOpaque,
				}, secret)
			},
		},
		{
			name:      "create cronjob with service account annotation",
			jobName:   "myjob-1cec0fa1", // Hash-based name for "* * * * *"
			namespace: "default",
			scenario: func() {
				cj := jobTypes.Job{
					Name:      "myjob",
					TeamOwner: s.team.Name,
					Pool:      "test-default",
					Metadata: app.Metadata{
						Annotations: []app.MetadataItem{
							{
								Name:  AnnotationServiceAccountJobAnnotations,
								Value: `{"iam.gke.io/gcp-service-account":"test@test.com"}`,
							},
						},
					},
					Spec: jobTypes.JobSpec{
						Schedule: "* * * * *",
						Container: jobTypes.ContainerInfo{
							OriginalImageSrc: "ubuntu:latest",
							Command:          []string{"echo", "hello world"},
						},
					},
				}
				err := s.p.EnsureJob(context.TODO(), &cj)
				waitCron()
				require.NoError(s.t, err)
			},
			assertion: func(gotCron *batchv1.CronJob) {
				jobName := "myjob" // original job name
				serviceAccountName := "job-" + jobName
				account, err := s.client.CoreV1().ServiceAccounts(gotCron.Namespace).Get(context.TODO(), serviceAccountName, metav1.GetOptions{})
				require.NoError(s.t, err)
				require.EqualValues(s.t, &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceAccountName,
						Namespace: gotCron.Namespace,
						Labels: map[string]string{
							"tsuru.io/is-tsuru": "true",
							"tsuru.io/job-name": jobName,
						},
						Annotations: map[string]string{
							"iam.gke.io/gcp-service-account": "test@test.com",
						},
					},
				}, account)
			},
			teardown: func() {
				j := jobTypes.Job{
					Name: "myjob",
					Pool: "test-default",
					Spec: jobTypes.JobSpec{
						Schedule: "* * * * *",
					},
				}
				err := s.p.DestroyJob(context.TODO(), &j)
				require.NoError(s.t, err)

				_, err = s.client.BatchV1().CronJobs("default").Get(context.TODO(), "myjob-1cec0fa1", metav1.GetOptions{})
				require.True(s.t, k8sErrors.IsNotFound(err))
			},
		},
		{
			name:      "create cronjob with concurrency policy set to forbid",
			jobName:   "myjob-1cec0fa1", // Hash-based name for "* * * * *"
			namespace: "default",
			scenario: func() {
				cj := jobTypes.Job{
					Name:      "myjob",
					TeamOwner: s.team.Name,
					Pool:      "test-default",
					Spec: jobTypes.JobSpec{
						ConcurrencyPolicy: func() *string { r := "Forbid"; return &r }(),
						Schedule:          "* * * * *",
						Container: jobTypes.ContainerInfo{
							OriginalImageSrc: "ubuntu:latest",
							Command:          []string{"echo", "hello world"},
						},
					},
				}
				err := s.p.EnsureJob(context.TODO(), &cj)
				waitCron()
				require.NoError(s.t, err)
			},
			assertion: func(gotCron *batchv1.CronJob) {
				require.EqualValues(s.t, batchv1.ForbidConcurrent, gotCron.Spec.ConcurrencyPolicy)
			},
			teardown: func() {
				j := jobTypes.Job{
					Name: "myjob",
					Pool: "test-default",
					Spec: jobTypes.JobSpec{
						Schedule: "* * * * *",
					},
				}
				err := s.p.DestroyJob(context.TODO(), &j)
				require.NoError(s.t, err)

				_, err = s.client.BatchV1().CronJobs("default").Get(context.TODO(), "myjob-1cec0fa1", metav1.GetOptions{})
				require.True(s.t, k8sErrors.IsNotFound(err))
			},
		},
	}
	for _, tt := range tests {
		tt.scenario()
		gotCron, err := s.client.BatchV1().CronJobs(tt.namespace).Get(context.TODO(), tt.jobName, metav1.GetOptions{})
		require.NoError(s.t, err)
		tt.assertion(gotCron)
		tt.teardown()
	}
}

func (s *S) TestProvisionerUpdateCronJob(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	tests := []struct {
		name           string
		setup          func()
		scenario       func()
		expectedTarget *batchv1.CronJob
	}{
		{
			name: "simple update cronjob",
			setup: func() {
				cj := jobTypes.Job{
					Name:      "myjob",
					TeamOwner: s.team.Name,
					Pool:      "test-default",
					Metadata: app.Metadata{
						Labels: []app.MetadataItem{
							{
								Name:  "label1",
								Value: "value1",
							},
						},
						Annotations: []app.MetadataItem{
							{
								Name:  "annotation1",
								Value: "value2",
							},
						},
					},
					Spec: jobTypes.JobSpec{
						Schedule:              "* * * * *",
						Parallelism:           func() *int32 { r := int32(3); return &r }(),
						Completions:           func() *int32 { r := int32(1); return &r }(),
						ActiveDeadlineSeconds: func() *int64 { r := int64(5 * 60); return &r }(),
						BackoffLimit:          func() *int32 { r := int32(7); return &r }(),
						Container: jobTypes.ContainerInfo{
							OriginalImageSrc: "ubuntu:latest",
							Command:          []string{"echo", "hello world"},
						},
					},
				}
				err := s.p.EnsureJob(context.TODO(), &cj)
				waitCron()
				require.NoError(s.t, err)
			},
			scenario: func() {
				newCJ := jobTypes.Job{
					Name:      "myjob",
					TeamOwner: s.team.Name,
					Pool:      "test-default",
					Plan:      app.Plan{Name: "c4m2"},
					Metadata: app.Metadata{
						Labels: []app.MetadataItem{
							{
								Name:  "label2",
								Value: "value3",
							},
						},
						Annotations: []app.MetadataItem{
							{
								Name:  "annotation2",
								Value: "value4",
							},
						},
					},
					Spec: jobTypes.JobSpec{
						Schedule:              "* * * * *",
						ConcurrencyPolicy:     func() *string { r := "Forbid"; return &r }(),
						Parallelism:           func() *int32 { r := int32(2); return &r }(),
						Completions:           func() *int32 { r := int32(1); return &r }(),
						ActiveDeadlineSeconds: func() *int64 { r := int64(0); return &r }(),
						BackoffLimit:          func() *int32 { r := int32(6); return &r }(),
						Container: jobTypes.ContainerInfo{
							OriginalImageSrc: "ubuntu:latest",
							Command:          []string{"echo", "hello world"},
						},
					},
				}
				err := s.p.EnsureJob(context.TODO(), &newCJ)
				waitCron()
				require.NoError(s.t, err)
			},
			expectedTarget: &batchv1.CronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myjob-1cec0fa1", // Hash-based name for "* * * * *"
					Namespace: "default",
					Labels: map[string]string{
						"app.kubernetes.io/component":  "job",
						"app.kubernetes.io/managed-by": "tsuru",
						"app.kubernetes.io/name":       "tsuru-job",
						"app.kubernetes.io/instance":   "myjob",
						"tsuru.io/is-tsuru":            "true",
						"tsuru.io/is-service":          "true",
						"tsuru.io/job-name":            "myjob",
						"tsuru.io/job-pool":            "test-default",
						"tsuru.io/job-team":            "admin",
						"tsuru.io/is-job":              "true",
						"tsuru.io/job-manual":          "false",
						"tsuru.io/is-build":            "false",
						"label2":                       "value3",
					},
					Annotations: map[string]string{"annotation2": "value4"},
				},
				Spec: batchv1.CronJobSpec{
					Schedule:          "* * * * *",
					ConcurrencyPolicy: batchv1.ForbidConcurrent,
					Suspend:           func() *bool { r := false; return &r }(),
					JobTemplate: batchv1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							TTLSecondsAfterFinished: func() *int32 { defaultTTL := int32(86400); return &defaultTTL }(),
							Parallelism:             func() *int32 { r := int32(2); return &r }(),
							Completions:             func() *int32 { r := int32(1); return &r }(),
							ActiveDeadlineSeconds:   func() *int64 { r := int64(60 * 60); return &r }(),
							BackoffLimit:            func() *int32 { r := int32(6); return &r }(),
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: map[string]string{
										"app.kubernetes.io/component":  "job",
										"app.kubernetes.io/managed-by": "tsuru",
										"app.kubernetes.io/name":       "tsuru-job",
										"app.kubernetes.io/instance":   "myjob",
										"tsuru.io/is-tsuru":            "true",
										"tsuru.io/is-service":          "true",
										"tsuru.io/job-name":            "myjob",
										"tsuru.io/job-pool":            "test-default",
										"tsuru.io/job-team":            "admin",
										"tsuru.io/is-job":              "true",
										"tsuru.io/job-manual":          "false",
										"tsuru.io/is-build":            "false",
										"label2":                       "value3",
									},
									Annotations: map[string]string{"annotation2": "value4"},
								},
								Spec: corev1.PodSpec{
									ServiceAccountName: "job-myjob",
									Containers: []corev1.Container{
										{
											Name:    "job",
											Image:   "ubuntu:latest",
											Command: []string{"echo", "hello world"},
											Env:     []corev1.EnvVar{},
											Resources: corev1.ResourceRequirements{
												Limits: corev1.ResourceList{
													corev1.ResourceEphemeralStorage: resource.MustParse("100Mi"),
												},
												Requests: corev1.ResourceList{
													corev1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
												},
											},
										},
									},
									RestartPolicy: "OnFailure",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "changing the schedule cronjob",
			setup: func() {
				cj := jobTypes.Job{
					Name:      "myjob",
					TeamOwner: s.team.Name,
					Pool:      "test-default",
					Spec: jobTypes.JobSpec{
						Schedule: "* * * * *",
						Container: jobTypes.ContainerInfo{
							OriginalImageSrc: "ubuntu:latest",
							Command:          []string{"echo", "hello world"},
						},
					},
				}
				err := s.p.EnsureJob(context.TODO(), &cj)
				waitCron()
				require.NoError(s.t, err)
			},
			scenario: func() {
				newCJ := jobTypes.Job{
					Name:      "myjob",
					TeamOwner: s.team.Name,
					Pool:      "test-default",
					Plan:      app.Plan{Name: "c4m2"},
					Spec: jobTypes.JobSpec{
						Schedule: "*/2 * * * *",
						Container: jobTypes.ContainerInfo{
							OriginalImageSrc: "ubuntu:latest",
							Command:          []string{"echo", "hello world"},
						},
					},
				}
				err := s.p.EnsureJob(context.TODO(), &newCJ)
				waitCron()
				require.NoError(s.t, err)
			},
			expectedTarget: &batchv1.CronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myjob-e5fcddb0", // Hash-based name for "*/2 * * * *"
					Namespace: "default",
					Labels: map[string]string{
						"app.kubernetes.io/component":  "job",
						"app.kubernetes.io/managed-by": "tsuru",
						"app.kubernetes.io/name":       "tsuru-job",
						"app.kubernetes.io/instance":   "myjob",
						"tsuru.io/is-tsuru":            "true",
						"tsuru.io/is-service":          "true",
						"tsuru.io/job-name":            "myjob",
						"tsuru.io/job-pool":            "test-default",
						"tsuru.io/job-team":            "admin",
						"tsuru.io/is-job":              "true",
						"tsuru.io/job-manual":          "false",
						"tsuru.io/is-build":            "false",
					},
					Annotations: map[string]string{},
				},
				Spec: batchv1.CronJobSpec{
					Schedule: "*/2 * * * *",
					Suspend:  ptr.To[bool](false),
					JobTemplate: batchv1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							TTLSecondsAfterFinished: func() *int32 { defaultTTL := int32(86400); return &defaultTTL }(),
							ActiveDeadlineSeconds:   func() *int64 { r := int64(60 * 60); return &r }(),
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: map[string]string{
										"app.kubernetes.io/component":  "job",
										"app.kubernetes.io/managed-by": "tsuru",
										"app.kubernetes.io/name":       "tsuru-job",
										"app.kubernetes.io/instance":   "myjob",
										"tsuru.io/is-tsuru":            "true",
										"tsuru.io/is-service":          "true",
										"tsuru.io/job-name":            "myjob",
										"tsuru.io/job-pool":            "test-default",
										"tsuru.io/job-team":            "admin",
										"tsuru.io/is-job":              "true",
										"tsuru.io/job-manual":          "false",
										"tsuru.io/is-build":            "false",
									},
									Annotations: map[string]string{},
								},
								Spec: corev1.PodSpec{
									ServiceAccountName: "job-myjob",
									Containers: []corev1.Container{
										{
											Name:    "job",
											Image:   "ubuntu:latest",
											Command: []string{"echo", "hello world"},
											Env:     []corev1.EnvVar{},
											Resources: corev1.ResourceRequirements{
												Limits: corev1.ResourceList{
													corev1.ResourceEphemeralStorage: resource.MustParse("100Mi"),
												},
												Requests: corev1.ResourceList{
													corev1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
												},
											},
										},
									},
									RestartPolicy: "OnFailure",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		tt.setup()
		tt.scenario()
		gotCron, err := s.client.BatchV1().CronJobs(tt.expectedTarget.Namespace).Get(context.TODO(), tt.expectedTarget.Name, metav1.GetOptions{})
		require.NoError(s.t, err)
		require.EqualValues(s.t, *tt.expectedTarget, *gotCron)

		j := jobTypes.Job{
			Name: "myjob",
			Pool: "test-default",
			Spec: jobTypes.JobSpec{
				Schedule: "* * * * *",
			},
		}
		err = s.p.DestroyJob(context.TODO(), &j)
		require.NoError(s.t, err)
	}
}

func (s *S) TestProvisionerDeleteCronjob(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()
	cj := jobTypes.Job{
		Name:      "mycronjob",
		TeamOwner: s.team.Name,
		Pool:      "test-default",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "ubuntu:latest",
				Command:          []string{"echo", "hello world"},
			},
		},
	}
	tests := []struct {
		name         string
		setup        func()
		scenario     func()
		testScenario func(c *check.C)
	}{
		{
			name: "simple delete cronjob",
			setup: func() {
				err := s.p.EnsureJob(context.TODO(), &cj)
				waitCron()
				require.NoError(s.t, err)
			},
			scenario: func() {
				err := s.p.DestroyJob(context.TODO(), &cj)
				require.NoError(s.t, err)
				waitCron()
			},
			testScenario: func(c *check.C) {
				_, err := s.client.BatchV1().CronJobs("default").Get(context.TODO(), "mycronjob", metav1.GetOptions{})
				require.Error(s.t, err)
				require.True(s.t, k8sErrors.IsNotFound(err))
			},
		},
	}
	for _, tt := range tests {
		tt.setup()
		tt.scenario()
		tt.testScenario(c)
	}
}

func (s *S) TestProvisionerTriggerCron(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	cj := jobTypes.Job{
		Name:      "myjob",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "ubuntu:latest",
				Command:          []string{"echo", "hello world"},
			},
			Envs: []bindTypes.EnvVar{
				{
					Name:  "MY_ENV",
					Value: "** value",
				},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{
					ServiceName:  "database-as-service",
					InstanceName: "my-redis",
					EnvVar: bindTypes.EnvVar{
						Name:  "REDIS_HOST",
						Value: "localhost",
					},
				},
			},
		},
	}

	tests := []struct {
		name         string
		setup        func()
		scenario     func(t *time.Time)
		testScenario func(c *check.C, t *time.Time)
	}{
		{
			name: "simple trigger cronjob",
			setup: func() {
				err := s.p.EnsureJob(context.TODO(), &cj)
				waitCron()
				require.NoError(s.t, err)
			},
			scenario: func(t *time.Time) {
				*t = time.Now()
				err := s.p.TriggerCron(context.TODO(), &cj, "test-default")
				require.NoError(s.t, err)
				waitCron()
			},
			testScenario: func(c *check.C, t *time.Time) {
				expectedCronJobName := "myjob-1cec0fa1" // Hash-based name for "* * * * *"
				cronParent, err := s.client.BatchV1().CronJobs("default").Get(context.TODO(), expectedCronJobName, metav1.GetOptions{})
				require.NoError(s.t, err)
				expected := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-manual-job-%d", "myjob", t.Unix()/60),
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/component":  "job",
							"app.kubernetes.io/managed-by": "tsuru",
							"app.kubernetes.io/name":       "tsuru-job",
							"app.kubernetes.io/instance":   "myjob",
							"tsuru.io/is-tsuru":            "true",
							"tsuru.io/is-service":          "true",
							"tsuru.io/job-name":            "myjob",
							"tsuru.io/job-pool":            "pool1",
							"tsuru.io/job-team":            "admin",
							"tsuru.io/is-job":              "true",
							"tsuru.io/job-manual":          "false",
							"tsuru.io/is-build":            "false",
						},
						Annotations: map[string]string{"cronjob.kubernetes.io/instantiate": "manual"},
						OwnerReferences: []metav1.OwnerReference{
							{
								Name:       cronParent.Name,
								Kind:       "CronJob",
								UID:        cronParent.UID,
								APIVersion: "batch/v1",
							},
						},
					},
					Spec: batchv1.JobSpec{
						TTLSecondsAfterFinished: func() *int32 { defaultTTL := int32(86400); return &defaultTTL }(),
						ActiveDeadlineSeconds:   func() *int64 { r := int64(60 * 60); return &r }(),
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app.kubernetes.io/component":  "job",
									"app.kubernetes.io/managed-by": "tsuru",
									"app.kubernetes.io/name":       "tsuru-job",
									"app.kubernetes.io/instance":   "myjob",
									"tsuru.io/is-tsuru":            "true",
									"tsuru.io/is-service":          "true",
									"tsuru.io/job-name":            "myjob",
									"tsuru.io/job-pool":            "pool1",
									"tsuru.io/job-team":            "admin",
									"tsuru.io/is-job":              "true",
									"tsuru.io/job-manual":          "false",
									"tsuru.io/is-build":            "false",
								},
								Annotations: make(map[string]string),
							},
							Spec: corev1.PodSpec{
								ServiceAccountName: "job-myjob",
								Containers: []corev1.Container{
									{
										Name:    "job",
										Image:   "ubuntu:latest",
										Command: []string{"echo", "hello world"},
										Env: []corev1.EnvVar{
											{
												Name: "MY_ENV",
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														Key: "MY_ENV",
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "tsuru-job-myjob",
														},
													},
												},
											},
											{
												Name: "REDIS_HOST",
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														Key: "REDIS_HOST",
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "tsuru-job-myjob",
														},
													},
												},
											},
										},
										Resources: corev1.ResourceRequirements{
											Limits: corev1.ResourceList{
												corev1.ResourceEphemeralStorage: resource.MustParse("100Mi"),
											},
											Requests: corev1.ResourceList{
												corev1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
											},
										},
									},
								},
								RestartPolicy: "OnFailure",
							},
						},
					},
				}
				listJobs, err := s.client.BatchV1().Jobs(expected.Namespace).List(context.TODO(), metav1.ListOptions{})
				require.NoError(s.t, err)
				require.Len(s.t, listJobs.Items, 1)
				gotJob, err := s.client.BatchV1().Jobs(expected.Namespace).Get(context.TODO(), expected.Name, metav1.GetOptions{})
				require.NoError(s.t, err)
				require.EqualValues(s.t, expected, gotJob)
				// cleanup
				err = s.client.BatchV1().CronJobs(expected.Namespace).Delete(context.TODO(), cronParent.Name, metav1.DeleteOptions{})
				require.NoError(s.t, err)
			},
		},
	}
	for _, tt := range tests {
		var t time.Time
		tt.setup()
		tt.scenario(&t)
		tt.testScenario(c, &t)
	}
}

func (s *S) TestProvisionerTriggerCronDuplicate(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	cj := jobTypes.Job{
		Name:      "myjob",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "ubuntu:latest",
				Command:          []string{"echo", "hello world"},
			},
		},
	}

	// Create the cronjob first
	err := s.p.EnsureJob(context.TODO(), &cj)
	waitCron()
	require.NoError(s.t, err)

	// Trigger it once - should succeed
	err = s.p.TriggerCron(context.TODO(), &cj, "test-default")
	require.NoError(s.t, err)
	waitCron()

	// Trigger it again in the same minute - should get a better error message
	err = s.p.TriggerCron(context.TODO(), &cj, "test-default")
	require.Error(s.t, err)
	c.Assert(err.Error(), check.Matches, `.*manual job .* already exists.*once per minute.*`)
}

func (s *S) TestBackwardCompatibilityOldNaming(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	oldCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-job",
			Namespace: "default",
			Labels: map[string]string{
				"tsuru.io/job-name": "legacy-job",
				"tsuru.io/job-pool": "test-default",
				"tsuru.io/job-team": "admin",
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "0 2 * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:    "job",
									Image:   "ubuntu:latest",
									Command: []string{"echo", "legacy job"},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := s.client.BatchV1().CronJobs("default").Create(context.TODO(), oldCronJob, metav1.CreateOptions{})
	require.NoError(s.t, err)

	job := &jobTypes.Job{
		Name: "legacy-job",
		Pool: "test-default",
		Spec: jobTypes.JobSpec{
			Schedule: "0 2 * * *",
		},
	}

	err = s.p.TriggerCron(context.TODO(), job, "test-default")
	require.NoError(s.t, err)
	waitCron()

	jobs, err := s.client.BatchV1().Jobs("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, jobs.Items, 1)
	require.Contains(s.t, jobs.Items[0].Name, "legacy-job-manual-job-")

	err = s.p.DestroyJob(context.TODO(), job)
	require.NoError(s.t, err)

	_, err = s.client.BatchV1().CronJobs("default").Get(context.TODO(), "legacy-job", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
}

func (s *S) TestScheduleChangeTriggersNameMigration(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	job := &jobTypes.Job{
		Name:      "schedule-change-job",
		TeamOwner: s.team.Name,
		Pool:      "test-default",
		Spec: jobTypes.JobSpec{
			Schedule: "0 1 * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "ubuntu:latest",
				Command:          []string{"echo", "original schedule"},
			},
		},
	}

	err := s.p.EnsureJob(context.TODO(), job)
	waitCron()
	require.NoError(s.t, err)

	originalHash := "26f58082" // Hash of "0 1 * * *"
	originalName := fmt.Sprintf("schedule-change-job-%s", originalHash)

	originalCronJob, err := s.client.BatchV1().CronJobs("default").Get(context.TODO(), originalName, metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "0 1 * * *", originalCronJob.Spec.Schedule)

	job.Spec.Schedule = "0 2 * * *"
	err = s.p.EnsureJob(context.TODO(), job)
	waitCron()
	require.NoError(s.t, err)

	newHash := "746248f8" // Hash of "0 2 * * *"
	newName := fmt.Sprintf("schedule-change-job-%s", newHash)

	newCronJob, err := s.client.BatchV1().CronJobs("default").Get(context.TODO(), newName, metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "0 2 * * *", newCronJob.Spec.Schedule)

	_, err = s.client.BatchV1().CronJobs("default").Get(context.TODO(), originalName, metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	err = s.p.DestroyJob(context.TODO(), job)
	require.NoError(s.t, err)
}

func (s *S) TestMixedOldAndNewJobsCoexisting(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	oldCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "old-style-job",
			Namespace: "default",
			Labels: map[string]string{
				"tsuru.io/job-name": "old-style-job",
				"tsuru.io/job-pool": "test-default",
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "0 3 * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:    "job",
									Image:   "ubuntu:latest",
									Command: []string{"echo", "old style"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, err := s.client.BatchV1().CronJobs("default").Create(context.TODO(), oldCronJob, metav1.CreateOptions{})
	require.NoError(s.t, err)

	newJob := &jobTypes.Job{
		Name:      "new-style-job",
		TeamOwner: s.team.Name,
		Pool:      "test-default",
		Spec: jobTypes.JobSpec{
			Schedule: "0 4 * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "ubuntu:latest",
				Command:          []string{"echo", "new style"},
			},
		},
	}

	err = s.p.EnsureJob(context.TODO(), newJob)
	waitCron()
	require.NoError(s.t, err)

	cronJobs, err := s.client.BatchV1().CronJobs("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, cronJobs.Items, 2)

	var foundOld, foundNew *batchv1.CronJob
	for i := range cronJobs.Items {
		if cronJobs.Items[i].Name == "old-style-job" {
			foundOld = &cronJobs.Items[i]
		}
		if cronJobs.Items[i].Name == "new-style-job-0ba863fe" { // Hash-based name for "0 4 * * *"
			foundNew = &cronJobs.Items[i]
		}
	}

	require.NotNil(s.t, foundOld)
	require.NotNil(s.t, foundNew)
	require.Equal(s.t, "0 3 * * *", foundOld.Spec.Schedule)
	require.Equal(s.t, "0 4 * * *", foundNew.Spec.Schedule)

	oldJobSpec := &jobTypes.Job{Name: "old-style-job", Pool: "test-default", Spec: jobTypes.JobSpec{Schedule: "0 3 * * *"}}
	err = s.p.TriggerCron(context.TODO(), oldJobSpec, "test-default")
	require.NoError(s.t, err)

	err = s.p.TriggerCron(context.TODO(), newJob, "test-default")
	require.NoError(s.t, err)
	waitCron()

	jobs, err := s.client.BatchV1().Jobs("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, jobs.Items, 2)

	err = s.p.DestroyJob(context.TODO(), oldJobSpec)
	require.NoError(s.t, err)
	err = s.p.DestroyJob(context.TODO(), newJob)
	require.NoError(s.t, err)
}

func (s *S) TestCreateJobEvent(c *check.C) {
	boolTrue := true
	cleanup := func() {
		err := storagev2.ClearAllCollections(nil)
		require.NoError(s.t, err)
	}
	tests := []struct {
		name         string
		scenario     func()
		testScenario func(c *check.C)
	}{
		{
			name: "when job and evt (jobrun) are valid and job has cronjob owner reference",
			scenario: func() {
				j := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob",
						Namespace: "default",
						Labels: map[string]string{
							"tsuru.io/is-tsuru": "true",
							"tsuru.io/job-name": "myjob",
							"tsuru.io/job-pool": "test-default",
							"tsuru.io/job-team": "admin",
							"tsuru.io/is-job":   "true",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "batch/v1",
								Controller:         &boolTrue,
								BlockOwnerDeletion: &boolTrue,
								Kind:               "CronJob",
								Name:               "cronjob-parent",
								UID:                k8sTypes.UID("1234"),
							},
						},
					},
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"tsuru.io/is-tsuru": "true",
									"tsuru.io/job-name": "myjob",
									"tsuru.io/job-pool": "test-default",
									"tsuru.io/job-team": "admin",
									"tsuru.io/is-job":   "true",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:    "job",
										Image:   "ubuntu:latest",
										Command: []string{"echo", "hello world"},
									},
								},
								RestartPolicy: "OnFailure",
							},
						},
					},
				}
				evt := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob-somehash",
						Namespace: j.Namespace,
					},
					InvolvedObject: corev1.ObjectReference{
						APIVersion:      "batch/v1",
						Kind:            "Job",
						Name:            j.Name,
						Namespace:       j.Namespace,
						ResourceVersion: j.ResourceVersion,
						UID:             j.UID,
					},
					Message: "Job completed",
					Reason:  "Completed",
					Type:    "Normal",
				}
				wg := &sync.WaitGroup{}
				wg.Add(1)
				createJobEvent(s.clusterClient, j, evt, wg)
			},
			testScenario: func(c *check.C) {
				evts, err := event.List(context.TODO(), &event.Filter{})
				require.NoError(s.t, err)
				require.Len(s.t, evts, 1)
				gotEvt := evts[0]
				require.NotNil(s.t, gotEvt)
				require.EqualValues(s.t, eventTypes.Target{Type: eventTypes.TargetTypeJob, Value: "cronjob-parent"}, gotEvt.Target)
				require.EqualValues(s.t, event.Allowed(permission.PermJobReadEvents, permission.Context(permTypes.CtxJob, "cronjob-parent")), gotEvt.Allowed)
				require.EqualValues(s.t, eventTypes.OwnerTypeInternal, gotEvt.Owner.Type)
				require.False(s.t, gotEvt.Cancelable)
				expectedCustomData := map[string]string{
					"job-name":           "myjob",
					"job-controller":     "cronjob-parent",
					"event-type":         "Normal",
					"event-reason":       "Completed",
					"message":            "Job completed",
					"cluster-start-time": metav1.Time{}.String(),
				}
				gotCustomData := map[string]string{}
				err = gotEvt.EndCustomData.Unmarshal(&gotCustomData)
				require.NoError(s.t, err)
				require.EqualValues(s.t, expectedCustomData, gotCustomData)
				require.Equal(s.t, "", gotEvt.Error)
			},
		},
		{
			name: "when job and evt (backofflimitexceeded) are valid",
			scenario: func() {
				j := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob",
						Namespace: "default",
						Labels: map[string]string{
							"tsuru.io/is-tsuru": "true",
							"tsuru.io/job-name": "myjob",
							"tsuru.io/job-pool": "test-default",
							"tsuru.io/job-team": "admin",
							"tsuru.io/is-job":   "true",
						},
					},
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"tsuru.io/is-tsuru": "true",
									"tsuru.io/job-name": "myjob",
									"tsuru.io/job-pool": "test-default",
									"tsuru.io/job-team": "admin",
									"tsuru.io/is-job":   "true",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:    "job",
										Image:   "ubuntu:latest",
										Command: []string{"echo", "hello world"},
									},
								},
								RestartPolicy: "OnFailure",
							},
						},
					},
				}
				evt := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob-somehash",
						Namespace: j.Namespace,
					},
					InvolvedObject: corev1.ObjectReference{
						APIVersion:      "batch/v1",
						Kind:            "Job",
						Name:            j.Name,
						Namespace:       j.Namespace,
						ResourceVersion: j.ResourceVersion,
						UID:             j.UID,
					},
					Message: "Job has reached the specified backoff limit",
					Reason:  "BackoffLimitExceeded",
					Type:    "Warning",
				}
				wg := &sync.WaitGroup{}
				wg.Add(1)
				createJobEvent(s.clusterClient, j, evt, wg)
			},
			testScenario: func(c *check.C) {
				evts, err := event.List(context.TODO(), &event.Filter{})
				require.NoError(s.t, err)
				require.Len(s.t, evts, 1)
				gotEvt := evts[0]
				require.NotNil(s.t, gotEvt)
				require.EqualValues(s.t, eventTypes.Target{Type: eventTypes.TargetTypeJob, Value: "myjob"}, gotEvt.Target)
				require.EqualValues(s.t, event.Allowed(permission.PermJobReadEvents, permission.Context(permTypes.CtxJob, "myjob")), gotEvt.Allowed)
				require.EqualValues(s.t, eventTypes.OwnerTypeInternal, gotEvt.Owner.Type)
				require.False(s.t, gotEvt.Cancelable)
				expectedCustomData := map[string]string{
					"job-name":           "myjob",
					"job-controller":     "myjob",
					"event-type":         "Warning",
					"event-reason":       "BackoffLimitExceeded",
					"message":            "Job has reached the specified backoff limit",
					"cluster-start-time": metav1.Time{}.String(),
				}
				gotCustomData := map[string]string{}
				err = gotEvt.EndCustomData.Unmarshal(&gotCustomData)
				require.NoError(s.t, err)
				require.EqualValues(s.t, expectedCustomData, gotCustomData)
			},
		},
		{
			name: "when evt reason does not apply",
			scenario: func() {
				j := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob",
						Namespace: "default",
						Labels: map[string]string{
							"tsuru.io/is-tsuru": "true",
							"tsuru.io/job-name": "myjob",
							"tsuru.io/job-pool": "test-default",
							"tsuru.io/job-team": "admin",
							"tsuru.io/is-job":   "true",
						},
					},
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"tsuru.io/is-tsuru": "true",
									"tsuru.io/job-name": "myjob",
									"tsuru.io/job-pool": "test-default",
									"tsuru.io/job-team": "admin",
									"tsuru.io/is-job":   "true",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:    "job",
										Image:   "ubuntu:latest",
										Command: []string{"echo", "hello world"},
									},
								},
								RestartPolicy: "OnFailure",
							},
						},
					},
				}
				evt := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob-somehash",
						Namespace: j.Namespace,
					},
					InvolvedObject: corev1.ObjectReference{
						APIVersion:      "batch/v1",
						Kind:            "Job",
						Name:            j.Name,
						Namespace:       j.Namespace,
						ResourceVersion: j.ResourceVersion,
						UID:             j.UID,
					},
					Message: "Some error message",
					Reason:  "SomeOtherReason",
					Type:    "Warning",
				}
				wg := &sync.WaitGroup{}
				wg.Add(1)
				createJobEvent(s.clusterClient, j, evt, wg)
			},
			testScenario: func(c *check.C) {
				evts, err := event.List(context.TODO(), &event.Filter{})
				require.NoError(s.t, err)
				require.Len(s.t, evts, 0)
			},
		},
		{
			name: "when job evt should expire in 1 hour",
			scenario: func() {
				j := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob",
						Namespace: "default",
						Labels: map[string]string{
							"tsuru.io/is-tsuru": "true",
							"tsuru.io/job-name": "myjob",
							"tsuru.io/job-pool": "test-default",
							"tsuru.io/job-team": "admin",
							"tsuru.io/is-job":   "true",
						},
					},
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"tsuru.io/is-tsuru": "true",
									"tsuru.io/job-name": "myjob",
									"tsuru.io/job-pool": "test-default",
									"tsuru.io/job-team": "admin",
									"tsuru.io/is-job":   "true",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:    "job",
										Image:   "ubuntu:latest",
										Command: []string{"echo", "hello world"},
									},
								},
								RestartPolicy: "OnFailure",
							},
						},
					},
				}
				cj := &batchv1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob",
						Namespace: "default",
					},
					Spec: batchv1.CronJobSpec{
						Schedule: "* * * * *",
						JobTemplate: batchv1.JobTemplateSpec{
							Spec: j.Spec,
							ObjectMeta: metav1.ObjectMeta{
								Labels:    j.Labels,
								Name:      j.Name,
								Namespace: j.Namespace,
							},
						},
					},
				}
				realCj, err := s.client.BatchV1().CronJobs("default").Create(context.TODO(), cj, metav1.CreateOptions{})
				require.NoError(s.t, err)
				defer func() {
					err := s.client.BatchV1().CronJobs("default").Delete(context.TODO(), cj.Name, metav1.DeleteOptions{})
					require.NoError(s.t, err)
				}()
				j.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion:         realCj.APIVersion,
						Controller:         &boolTrue,
						BlockOwnerDeletion: &boolTrue,
						Kind:               realCj.Kind,
						Name:               realCj.Name,
						UID:                realCj.UID,
					},
				}

				evt := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob-somehash",
						Namespace: j.Namespace,
					},
					InvolvedObject: corev1.ObjectReference{
						APIVersion:      "batch/v1",
						Kind:            "Job",
						Name:            j.Name,
						Namespace:       j.Namespace,
						ResourceVersion: j.ResourceVersion,
						UID:             j.UID,
					},
					Message: "Job completed",
					Reason:  "Completed",
					Type:    "Normal",
				}
				wg := &sync.WaitGroup{}
				wg.Add(1)
				createJobEvent(s.clusterClient, j, evt, wg)
			},
			testScenario: func(c *check.C) {
				evts, err := event.List(context.TODO(), &event.Filter{})
				require.NoError(s.t, err)
				require.Len(s.t, evts, 1)
				gotEvt := evts[0]
				require.NotNil(s.t, gotEvt)
				require.EqualValues(s.t, eventTypes.Target{Type: eventTypes.TargetTypeJob, Value: "myjob"}, gotEvt.Target)
				require.EqualValues(s.t, event.Allowed(permission.PermJobReadEvents, permission.Context(permTypes.CtxJob, "myjob")), gotEvt.Allowed)
				require.EqualValues(s.t, eventTypes.OwnerTypeInternal, gotEvt.Owner.Type)
				require.False(s.t, gotEvt.Cancelable)
				expectedCustomData := map[string]string{
					"job-name":           "myjob",
					"job-controller":     "myjob",
					"event-type":         "Normal",
					"event-reason":       "Completed",
					"message":            "Job completed",
					"cluster-start-time": metav1.Time{}.String(),
				}
				gotCustomData := map[string]string{}
				err = gotEvt.EndCustomData.Unmarshal(&gotCustomData)
				require.NoError(s.t, err)
				require.EqualValues(s.t, expectedCustomData, gotCustomData)
				require.Equal(s.t, "", gotEvt.Error)
				require.NotNil(s.t, gotEvt.ExpireAt)
				require.False(s.t, gotEvt.ExpireAt.After(time.Now().Add(61*time.Minute)))
				require.True(s.t, gotEvt.ExpireAt.After(time.Now().Add(59*time.Minute)))
			},
		},
		{
			name: "when job evt should expire at the default specified time",
			scenario: func() {
				j := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob",
						Namespace: "default",
						Labels: map[string]string{
							"tsuru.io/is-tsuru": "true",
							"tsuru.io/job-name": "myjob",
							"tsuru.io/job-pool": "test-default",
							"tsuru.io/job-team": "admin",
							"tsuru.io/is-job":   "true",
						},
					},
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"tsuru.io/is-tsuru": "true",
									"tsuru.io/job-name": "myjob",
									"tsuru.io/job-pool": "test-default",
									"tsuru.io/job-team": "admin",
									"tsuru.io/is-job":   "true",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:    "job",
										Image:   "ubuntu:latest",
										Command: []string{"echo", "hello world"},
									},
								},
								RestartPolicy: "OnFailure",
							},
						},
					},
				}
				cj := &batchv1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob",
						Namespace: "default",
					},
					Spec: batchv1.CronJobSpec{
						Schedule: "@dayly",
						JobTemplate: batchv1.JobTemplateSpec{
							Spec: j.Spec,
							ObjectMeta: metav1.ObjectMeta{
								Labels:    j.Labels,
								Name:      j.Name,
								Namespace: j.Namespace,
							},
						},
					},
				}
				realCj, err := s.client.BatchV1().CronJobs("default").Create(context.TODO(), cj, metav1.CreateOptions{})
				require.NoError(s.t, err)
				defer func() {
					err := s.client.BatchV1().CronJobs("default").Delete(context.TODO(), cj.Name, metav1.DeleteOptions{})
					require.NoError(s.t, err)
				}()
				j.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion:         realCj.APIVersion,
						Controller:         &boolTrue,
						BlockOwnerDeletion: &boolTrue,
						Kind:               realCj.Kind,
						Name:               realCj.Name,
						UID:                realCj.UID,
					},
				}

				evt := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myjob-somehash",
						Namespace: j.Namespace,
					},
					InvolvedObject: corev1.ObjectReference{
						APIVersion:      "batch/v1",
						Kind:            "Job",
						Name:            j.Name,
						Namespace:       j.Namespace,
						ResourceVersion: j.ResourceVersion,
						UID:             j.UID,
					},
					Message: "Job completed",
					Reason:  "Completed",
					Type:    "Normal",
				}
				wg := &sync.WaitGroup{}
				wg.Add(1)
				createJobEvent(s.clusterClient, j, evt, wg)
			},
			testScenario: func(c *check.C) {
				evts, err := event.List(context.TODO(), &event.Filter{})
				require.NoError(s.t, err)
				require.Len(s.t, evts, 1)
				gotEvt := evts[0]
				require.NotNil(s.t, gotEvt)
				require.EqualValues(s.t, eventTypes.Target{Type: eventTypes.TargetTypeJob, Value: "myjob"}, gotEvt.Target)
				require.EqualValues(s.t, event.Allowed(permission.PermJobReadEvents, permission.Context(permTypes.CtxJob, "myjob")), gotEvt.Allowed)
				require.EqualValues(s.t, eventTypes.OwnerTypeInternal, gotEvt.Owner.Type)
				require.False(s.t, gotEvt.Cancelable)
				expectedCustomData := map[string]string{
					"job-name":           "myjob",
					"job-controller":     "myjob",
					"event-type":         "Normal",
					"event-reason":       "Completed",
					"message":            "Job completed",
					"cluster-start-time": metav1.Time{}.String(),
				}
				gotCustomData := map[string]string{}
				err = gotEvt.EndCustomData.Unmarshal(&gotCustomData)
				require.NoError(s.t, err)
				require.EqualValues(s.t, expectedCustomData, gotCustomData)
				require.Equal(s.t, "", gotEvt.Error)
				require.NotNil(s.t, gotEvt.ExpireAt)
				require.False(s.t, gotEvt.ExpireAt.After(time.Now().Add(25*time.Hour)))
				require.True(s.t, gotEvt.ExpireAt.After(time.Now().Add(23*time.Hour)))
			},
		},
	}

	for _, tt := range tests {
		tt.scenario()
		tt.testScenario(c)
		cleanup()
	}
}

func (s *S) TestKillJobUnit(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()
	cj := jobTypes.Job{
		Name:      "myjob",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	err := s.p.EnsureJob(context.TODO(), &cj)
	defer func() {
		j := jobTypes.Job{
			Name: "myjob",
			Pool: "pool1",
		}
		err = s.p.DestroyJob(context.TODO(), &j)
		require.NoError(s.t, err)
	}()
	waitCron()
	require.NoError(s.t, err)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myjob-unit1",
			Namespace: "default",
			Labels: map[string]string{
				"tsuru.io/job-name": "myjob",
			},
		},
	}
	k8sJob, err := s.client.BatchV1().Jobs("default").Create(context.TODO(), job, metav1.CreateOptions{})
	require.NoError(s.t, err)
	require.NotNil(s.t, k8sJob)
	require.Equal(s.t, "myjob-unit1", k8sJob.Name)
	err = s.p.KillJobUnit(context.TODO(), &jobTypes.Job{Name: "myjob", Pool: "pool1"}, "myjob-unit1", false)
	require.NoError(s.t, err)
	_, err = s.client.BatchV1().Jobs("default").Get(context.TODO(), "myjob-unit1", metav1.GetOptions{})
	require.Error(s.t, err)
	require.True(s.t, k8sErrors.IsNotFound(err))
}

func (s *S) TestKillJobUnitUnknow(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()
	cj := jobTypes.Job{
		Name:      "myjob",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	err := s.p.EnsureJob(context.TODO(), &cj)
	defer func() {
		j := jobTypes.Job{
			Name: "myjob",
			Pool: "pool1",
		}
		err = s.p.DestroyJob(context.TODO(), &j)
		require.NoError(s.t, err)
	}()
	waitCron()
	require.NoError(s.t, err)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myjob-unit1",
			Namespace: "default",
			Labels: map[string]string{
				"tsuru.io/job-name": "myotherjob",
			},
		},
	}
	k8sJob, err := s.client.BatchV1().Jobs("default").Create(context.TODO(), job, metav1.CreateOptions{})
	require.NoError(s.t, err)
	require.NotNil(s.t, k8sJob)
	require.Equal(s.t, "myjob-unit1", k8sJob.Name)
	err = s.p.KillJobUnit(context.TODO(), &jobTypes.Job{Name: "myjob", Pool: "pool1"}, "myjob-unit1", false)
	require.Error(s.t, err)
	require.ErrorContains(s.t, err, `unit "myjob-unit1" not found`)
	_, err = s.client.BatchV1().Jobs("default").Get(context.TODO(), "myjob-unit1", metav1.GetOptions{})
	require.NoError(s.t, err)
}

func (s *S) TestFindJobFailedReason(c *check.C) {
	testCases := []struct {
		name           string
		job            *batchv1.Job
		expectedReason string
	}{
		{
			name: "Job with Failed Condition",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
							Reason: "BackoffLimitExceeded",
						},
					},
				},
			},
			expectedReason: "BackoffLimitExceeded",
		},
		{
			name: "Job without Failed Condition",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedReason: "",
		},
		{
			name: "Job with No Conditions",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{},
			},
			expectedReason: "",
		},
	}

	for _, tc := range testCases {
		reason := findJobFailedReason(tc.job)
		require.Equal(s.t, tc.expectedReason, reason)
	}
}

func (s *S) TestKillJobUnitWithPodCleanup(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()
	cj := jobTypes.Job{
		Name:      "myjob",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	err := s.p.EnsureJob(context.TODO(), &cj)
	defer func() {
		j := jobTypes.Job{
			Name: "myjob",
			Pool: "pool1",
		}
		err = s.p.DestroyJob(context.TODO(), &j)
		require.NoError(s.t, err)
	}()
	waitCron()
	require.NoError(s.t, err)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myjob-unit1",
			Namespace: "default",
			Labels: map[string]string{
				"tsuru.io/job-name": "myjob",
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:    "job",
							Image:   "ubuntu:latest",
							Command: []string{"echo", "hello world"},
						},
					},
				},
			},
		},
	}
	k8sJob, err := s.client.BatchV1().Jobs("default").Create(context.TODO(), job, metav1.CreateOptions{})
	require.NoError(s.t, err)
	require.NotNil(s.t, k8sJob)
	require.Equal(s.t, "myjob-unit1", k8sJob.Name)

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myjob-unit1-pod1",
			Namespace: "default",
			Labels: map[string]string{
				"job-name": "myjob-unit1",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Containers: []corev1.Container{
				{
					Name:    "job",
					Image:   "ubuntu:latest",
					Command: []string{"echo", "hello world"},
				},
			},
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myjob-unit1-pod2",
			Namespace: "default",
			Labels: map[string]string{
				"job-name": "myjob-unit1",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Containers: []corev1.Container{
				{
					Name:    "job",
					Image:   "ubuntu:latest",
					Command: []string{"echo", "hello world"},
				},
			},
		},
	}

	_, err = s.client.CoreV1().Pods("default").Create(context.TODO(), pod1, metav1.CreateOptions{})
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().Pods("default").Create(context.TODO(), pod2, metav1.CreateOptions{})
	require.NoError(s.t, err)

	pods, err := s.client.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "job-name=myjob-unit1",
	})
	require.NoError(s.t, err)
	require.Len(s.t, pods.Items, 2)

	err = s.p.KillJobUnit(context.TODO(), &jobTypes.Job{Name: "myjob", Pool: "pool1"}, "myjob-unit1", false)
	require.NoError(s.t, err)

	_, err = s.client.BatchV1().Jobs("default").Get(context.TODO(), "myjob-unit1", metav1.GetOptions{})
	require.Error(s.t, err)
	require.True(s.t, k8sErrors.IsNotFound(err))

	pods, err = s.client.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "job-name=myjob-unit1",
	})
	require.NoError(s.t, err)
	require.Len(s.t, pods.Items, 0)
}

func (s *S) TestKillJobUnitWithPodCleanupForced(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()
	cj := jobTypes.Job{
		Name:      "myjob",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	err := s.p.EnsureJob(context.TODO(), &cj)
	defer func() {
		j := jobTypes.Job{
			Name: "myjob",
			Pool: "pool1",
		}
		err = s.p.DestroyJob(context.TODO(), &j)
		require.NoError(s.t, err)
	}()
	waitCron()
	require.NoError(s.t, err)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myjob-unit1-forced",
			Namespace: "default",
			Labels: map[string]string{
				"tsuru.io/job-name": "myjob",
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:    "job",
							Image:   "ubuntu:latest",
							Command: []string{"echo", "hello world"},
						},
					},
				},
			},
		},
	}
	k8sJob, err := s.client.BatchV1().Jobs("default").Create(context.TODO(), job, metav1.CreateOptions{})
	require.NoError(s.t, err)
	require.NotNil(s.t, k8sJob)
	require.Equal(s.t, "myjob-unit1-forced", k8sJob.Name)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myjob-unit1-forced-pod",
			Namespace: "default",
			Labels: map[string]string{
				"job-name": "myjob-unit1-forced",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Containers: []corev1.Container{
				{
					Name:    "job",
					Image:   "ubuntu:latest",
					Command: []string{"echo", "hello world"},
				},
			},
		},
	}

	_, err = s.client.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
	require.NoError(s.t, err)

	pods, err := s.client.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "job-name=myjob-unit1-forced",
	})
	require.NoError(s.t, err)
	require.Len(s.t, pods.Items, 1)

	err = s.p.KillJobUnit(context.TODO(), &jobTypes.Job{Name: "myjob", Pool: "pool1"}, "myjob-unit1-forced", true)
	require.NoError(s.t, err)

	_, err = s.client.BatchV1().Jobs("default").Get(context.TODO(), "myjob-unit1-forced", metav1.GetOptions{})
	require.Error(s.t, err)
	require.True(s.t, k8sErrors.IsNotFound(err))

	pods, err = s.client.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "job-name=myjob-unit1-forced",
	})
	require.NoError(s.t, err)
	require.Len(s.t, pods.Items, 0)
}

func (s *S) TestScheduleChangeDeletesAllOldCronJobs(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	job := &jobTypes.Job{
		Name:      "multi-change-job",
		TeamOwner: s.team.Name,
		Pool:      "test-default",
		Spec: jobTypes.JobSpec{
			Schedule: "0 1 * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "ubuntu:latest",
				Command:          []string{"echo", "original"},
			},
		},
	}

	err := s.p.EnsureJob(context.TODO(), job)
	waitCron()
	require.NoError(s.t, err)

	initialName := generateJobNameWithScheduleHash(job)

	initialCronJob, err := s.client.BatchV1().CronJobs("default").Get(context.TODO(), initialName, metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "0 1 * * *", initialCronJob.Spec.Schedule)

	job.Spec.Schedule = "0 2 * * *"
	err = s.p.EnsureJob(context.TODO(), job)
	waitCron()
	require.NoError(s.t, err)

	secondName := generateJobNameWithScheduleHash(job)

	secondCronJob, err := s.client.BatchV1().CronJobs("default").Get(context.TODO(), secondName, metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "0 2 * * *", secondCronJob.Spec.Schedule)

	_, err = s.client.BatchV1().CronJobs("default").Get(context.TODO(), initialName, metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	job.Spec.Schedule = "0 3 * * *"
	err = s.p.EnsureJob(context.TODO(), job)
	waitCron()
	require.NoError(s.t, err)

	thirdName := generateJobNameWithScheduleHash(job)

	thirdCronJob, err := s.client.BatchV1().CronJobs("default").Get(context.TODO(), thirdName, metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "0 3 * * *", thirdCronJob.Spec.Schedule)

	_, err = s.client.BatchV1().CronJobs("default").Get(context.TODO(), secondName, metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	allCronJobs, err := s.client.BatchV1().CronJobs("default").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "tsuru.io/job-name=multi-change-job",
	})
	require.NoError(s.t, err)
	require.Len(s.t, allCronJobs.Items, 1)
	require.Equal(s.t, thirdName, allCronJobs.Items[0].Name)

	err = s.p.DestroyJob(context.TODO(), job)
	require.NoError(s.t, err)
}

func (s *S) TestMaxJobNameLengthWithHash(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	maxJobName := "this-is-a-very-long-job-name-exactly-40c"
	require.Len(s.t, maxJobName, 40)

	job := &jobTypes.Job{
		Name:      maxJobName,
		TeamOwner: s.team.Name,
		Pool:      "test-default",
		Spec: jobTypes.JobSpec{
			Schedule: "0 1 * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "ubuntu:latest",
				Command:          []string{"echo", "max length test"},
			},
		},
	}

	err := s.p.EnsureJob(context.TODO(), job)
	waitCron()
	require.NoError(s.t, err)

	expectedName := generateJobNameWithScheduleHash(job)
	require.Len(s.t, expectedName, 49)

	cronJob, err := s.client.BatchV1().CronJobs("default").Get(context.TODO(), expectedName, metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, expectedName, cronJob.Name)
	require.Equal(s.t, "0 1 * * *", cronJob.Spec.Schedule)

	job.Spec.Schedule = "0 2 * * *"
	err = s.p.EnsureJob(context.TODO(), job)
	waitCron()
	require.NoError(s.t, err)

	newExpectedName := generateJobNameWithScheduleHash(job)
	require.Len(s.t, newExpectedName, 49)

	newCronJob, err := s.client.BatchV1().CronJobs("default").Get(context.TODO(), newExpectedName, metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, newExpectedName, newCronJob.Name)
	require.Equal(s.t, "0 2 * * *", newCronJob.Spec.Schedule)

	_, err = s.client.BatchV1().CronJobs("default").Get(context.TODO(), expectedName, metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	err = s.p.TriggerCron(context.TODO(), job, "test-default")
	require.NoError(s.t, err)
	waitCron()

	manualJobs, err := s.client.BatchV1().Jobs("default").List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("tsuru.io/job-name=%s", maxJobName),
	})
	require.NoError(s.t, err)
	require.Len(s.t, manualJobs.Items, 1)

	err = s.p.DestroyJob(context.TODO(), job)
	require.NoError(s.t, err)
}
