// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/job"
	"github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
	check "gopkg.in/check.v1"
	batchv1 "k8s.io/api/batch/v1"
	apiv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) TestProvisionerCreateCronJob(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	tests := []struct {
		name           string
		scenario       func()
		expectedTarget *apiv1beta1.CronJob
	}{
		{
			name: "simple create cronjob",
			scenario: func() {
				cj := job.Job{
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
					Spec: job.JobSpec{
						Schedule:              "* * * * *",
						Parallelism:           func() *int32 { r := int32(3); return &r }(),
						Completions:           func() *int32 { r := int32(1); return &r }(),
						ActiveDeadlineSeconds: func() *int64 { r := int64(5 * 60); return &r }(),
						BackoffLimit:          func() *int32 { r := int32(7); return &r }(),
						Container: jobTypes.ContainerInfo{
							Name:    "c1",
							Image:   "ubuntu:latest",
							Command: []string{"echo", "hello world"},
						},
					},
				}
				_, err := s.p.CreateJob(context.TODO(), &cj)
				waitCron()
				c.Assert(err, check.IsNil)
			},
			expectedTarget: &apiv1beta1.CronJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "myjob",
					Namespace: "default",
					Labels: map[string]string{
						"job.kubernetes.io/component":  "tsuru-job",
						"job.kubernetes.io/managed-by": "tsuru",
						"job.kubernetes.io/name":       "myjob",
						"tsuru.io/is-tsuru":            "true",
						"tsuru.io/is-service":          "true",
						"tsuru.io/job-name":            "myjob",
						"tsuru.io/job-pool":            "test-default",
						"tsuru.io/job-team":            "admin",
						"tsuru.io/is-job":              "true",
						"label1":                       "value1",
					},
					Annotations: map[string]string{"annotation1": "value2"},
				},
				Spec: apiv1beta1.CronJobSpec{
					Schedule: "* * * * *",
					JobTemplate: apiv1beta1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							Parallelism:           func() *int32 { r := int32(3); return &r }(),
							Completions:           func() *int32 { r := int32(1); return &r }(),
							ActiveDeadlineSeconds: func() *int64 { r := int64(5 * 60); return &r }(),
							BackoffLimit:          func() *int32 { r := int32(7); return &r }(),
							Template: corev1.PodTemplateSpec{
								ObjectMeta: v1.ObjectMeta{
									Labels: map[string]string{
										"job.kubernetes.io/component":  "tsuru-job",
										"job.kubernetes.io/managed-by": "tsuru",
										"job.kubernetes.io/name":       "myjob",
										"tsuru.io/is-tsuru":            "true",
										"tsuru.io/is-service":          "true",
										"tsuru.io/job-name":            "myjob",
										"tsuru.io/job-pool":            "test-default",
										"tsuru.io/job-team":            "admin",
										"tsuru.io/is-job":              "true",
										"label1":                       "value1",
									},
									Annotations: map[string]string{"annotation1": "value2"},
								},
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name:    "c1",
											Image:   "ubuntu:latest",
											Command: []string{"echo", "hello world"},
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
		tt.scenario()
		gotCron, err := s.client.BatchV1beta1().CronJobs(tt.expectedTarget.Namespace).Get(context.TODO(), tt.expectedTarget.Name, v1.GetOptions{})
		c.Assert(err, check.IsNil)
		c.Assert(*gotCron, check.DeepEquals, *tt.expectedTarget)
	}
}

func (s *S) TestProvisionerCreateJob(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	tests := []struct {
		name           string
		scenario       func()
		expectedTarget *batchv1.Job
	}{
		{
			name: "simple create job",
			scenario: func() {
				j := job.Job{
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
					Spec: job.JobSpec{
						Parallelism:           func() *int32 { r := int32(3); return &r }(),
						Completions:           func() *int32 { r := int32(1); return &r }(),
						ActiveDeadlineSeconds: func() *int64 { r := int64(5 * 60); return &r }(),
						BackoffLimit:          func() *int32 { r := int32(7); return &r }(),
						Container: jobTypes.ContainerInfo{
							Name:    "c1",
							Image:   "ubuntu:latest",
							Command: []string{"echo", "hello world"},
						},
					},
				}
				_, err := s.p.CreateJob(context.TODO(), &j)
				waitCron()
				c.Assert(err, check.IsNil)
			},
			expectedTarget: &batchv1.Job{
				ObjectMeta: v1.ObjectMeta{
					Name:      "myjob",
					Namespace: "default",
					Labels: map[string]string{
						"job.kubernetes.io/component":  "tsuru-job",
						"job.kubernetes.io/managed-by": "tsuru",
						"job.kubernetes.io/name":       "myjob",
						"tsuru.io/is-tsuru":            "true",
						"tsuru.io/is-service":          "true",
						"tsuru.io/job-name":            "myjob",
						"tsuru.io/job-pool":            "test-default",
						"tsuru.io/job-team":            "admin",
						"tsuru.io/is-job":              "true",
						"label1":                       "value1",
					},
					Annotations: map[string]string{"annotation1": "value2"},
				},
				Spec: batchv1.JobSpec{
					Parallelism:           func() *int32 { r := int32(3); return &r }(),
					Completions:           func() *int32 { r := int32(1); return &r }(),
					ActiveDeadlineSeconds: func() *int64 { r := int64(5 * 60); return &r }(),
					BackoffLimit:          func() *int32 { r := int32(7); return &r }(),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{
								"job.kubernetes.io/component":  "tsuru-job",
								"job.kubernetes.io/managed-by": "tsuru",
								"job.kubernetes.io/name":       "myjob",
								"tsuru.io/is-tsuru":            "true",
								"tsuru.io/is-service":          "true",
								"tsuru.io/job-name":            "myjob",
								"tsuru.io/job-pool":            "test-default",
								"tsuru.io/job-team":            "admin",
								"tsuru.io/is-job":              "true",
								"label1":                       "value1",
							},
							Annotations: map[string]string{"annotation1": "value2"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:    "c1",
									Image:   "ubuntu:latest",
									Command: []string{"echo", "hello world"},
								},
							},
							RestartPolicy: "OnFailure",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		tt.scenario()
		gotJob, err := s.client.BatchV1().Jobs(tt.expectedTarget.Namespace).Get(context.TODO(), tt.expectedTarget.Name, v1.GetOptions{})
		c.Assert(err, check.IsNil)
		c.Assert(*gotJob, check.DeepEquals, *tt.expectedTarget)
	}
}

func (s *S) TestProvisionerUpdateCronJob(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	tests := []struct {
		name           string
		setup          func()
		scenario       func()
		expectedTarget *apiv1beta1.CronJob
	}{
		{
			name: "simple update cronjob",
			setup: func() {
				cj := job.Job{
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
					Spec: job.JobSpec{
						Schedule:              "* * * * *",
						Parallelism:           func() *int32 { r := int32(3); return &r }(),
						Completions:           func() *int32 { r := int32(1); return &r }(),
						ActiveDeadlineSeconds: func() *int64 { r := int64(5 * 60); return &r }(),
						BackoffLimit:          func() *int32 { r := int32(7); return &r }(),
						Container: jobTypes.ContainerInfo{
							Name:    "c1",
							Image:   "ubuntu:latest",
							Command: []string{"echo", "hello world"},
						},
					},
				}
				_, err := s.p.CreateJob(context.TODO(), &cj)
				waitCron()
				c.Assert(err, check.IsNil)
			},
			scenario: func() {
				newCJ := job.Job{
					Name:      "myjob",
					TeamOwner: s.team.Name,
					Pool:      "test-default",
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
					Spec: job.JobSpec{
						Schedule:              "* * * * *",
						Parallelism:           func() *int32 { r := int32(2); return &r }(),
						Completions:           func() *int32 { r := int32(1); return &r }(),
						ActiveDeadlineSeconds: func() *int64 { r := int64(4 * 60); return &r }(),
						BackoffLimit:          func() *int32 { r := int32(6); return &r }(),
						Container: jobTypes.ContainerInfo{
							Name:    "c1",
							Image:   "ubuntu:latest",
							Command: []string{"echo", "hello world"},
						},
					},
				}
				err := s.p.UpdateJob(context.TODO(), &newCJ)
				waitCron()
				c.Assert(err, check.IsNil)
			},
			expectedTarget: &apiv1beta1.CronJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "myjob",
					Namespace: "default",
					Labels: map[string]string{
						"job.kubernetes.io/component":  "tsuru-job",
						"job.kubernetes.io/managed-by": "tsuru",
						"job.kubernetes.io/name":       "myjob",
						"tsuru.io/is-tsuru":            "true",
						"tsuru.io/is-service":          "true",
						"tsuru.io/job-name":            "myjob",
						"tsuru.io/job-pool":            "test-default",
						"tsuru.io/job-team":            "admin",
						"tsuru.io/is-job":              "true",
						"label2":                       "value3",
					},
					Annotations: map[string]string{"annotation2": "value4"},
				},
				Spec: apiv1beta1.CronJobSpec{
					Schedule: "* * * * *",
					JobTemplate: apiv1beta1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							Parallelism:           func() *int32 { r := int32(2); return &r }(),
							Completions:           func() *int32 { r := int32(1); return &r }(),
							ActiveDeadlineSeconds: func() *int64 { r := int64(4 * 60); return &r }(),
							BackoffLimit:          func() *int32 { r := int32(6); return &r }(),
							Template: corev1.PodTemplateSpec{
								ObjectMeta: v1.ObjectMeta{
									Labels: map[string]string{
										"job.kubernetes.io/component":  "tsuru-job",
										"job.kubernetes.io/managed-by": "tsuru",
										"job.kubernetes.io/name":       "myjob",
										"tsuru.io/is-tsuru":            "true",
										"tsuru.io/is-service":          "true",
										"tsuru.io/job-name":            "myjob",
										"tsuru.io/job-pool":            "test-default",
										"tsuru.io/job-team":            "admin",
										"tsuru.io/is-job":              "true",
										"label2":                       "value3",
									},
									Annotations: map[string]string{"annotation2": "value4"},
								},
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name:    "c1",
											Image:   "ubuntu:latest",
											Command: []string{"echo", "hello world"},
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
		gotCron, err := s.client.BatchV1beta1().CronJobs(tt.expectedTarget.Namespace).Get(context.TODO(), tt.expectedTarget.Name, v1.GetOptions{})
		c.Assert(err, check.IsNil)
		c.Assert(*gotCron, check.DeepEquals, *tt.expectedTarget)
	}
}

func (s *S) TestProvisionerDeleteJob(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()
	cj := job.Job{
		Name:      "mycronjob",
		TeamOwner: s.team.Name,
		Pool:      "test-default",
		Spec: job.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				Name:    "c1",
				Image:   "ubuntu:latest",
				Command: []string{"echo", "hello world"},
			},
		},
	}
	j := job.Job{
		Name:      "myjob",
		TeamOwner: s.team.Name,
		Pool:      "test-default",
		Spec: job.JobSpec{
			Container: jobTypes.ContainerInfo{
				Name:    "c1",
				Image:   "ubuntu:latest",
				Command: []string{"echo", "hello world"},
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
				_, err := s.p.CreateJob(context.TODO(), &cj)
				waitCron()
				c.Assert(err, check.IsNil)
			},
			scenario: func() {
				err := s.p.DestroyJob(context.TODO(), &cj)
				c.Assert(err, check.IsNil)
				waitCron()
			},
			testScenario: func(c *check.C) {
				_, err := s.client.BatchV1beta1().CronJobs("default").Get(context.TODO(), "mycronjob", v1.GetOptions{})
				c.Assert(err, check.NotNil)
				c.Assert(k8sErrors.IsNotFound(err), check.Equals, true)
			},
		},
		{
			name: "simple delete job",
			setup: func() {
				_, err := s.p.CreateJob(context.TODO(), &j)
				c.Assert(err, check.IsNil)
			},
			scenario: func() {
				err := s.p.DestroyJob(context.TODO(), &j)
				c.Assert(err, check.IsNil)
			},
			testScenario: func(c *check.C) {
				_, err := s.client.BatchV1beta1().CronJobs("default").Get(context.TODO(), "myjob", v1.GetOptions{})
				c.Assert(err, check.NotNil)
				c.Assert(k8sErrors.IsNotFound(err), check.Equals, true)
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

	tests := []struct {
		name         string
		setup        func()
		scenario     func()
		testScenario func(c *check.C)
	}{
		{
			name: "simple trigger cronjob",
			setup: func() {
				cj := job.Job{
					Name:      "myjob",
					TeamOwner: s.team.Name,
					Pool:      "test-default",
					Spec: job.JobSpec{
						Schedule: "* * * * *",
						Container: jobTypes.ContainerInfo{
							Name:    "c1",
							Image:   "ubuntu:latest",
							Command: []string{"echo", "hello world"},
						},
					},
				}
				_, err := s.p.CreateJob(context.TODO(), &cj)
				waitCron()
				c.Assert(err, check.IsNil)
			},
			scenario: func() {
				err := s.p.TriggerCron(context.TODO(), "myjob", "test-default")
				c.Assert(err, check.IsNil)
				waitCron()
			},
			testScenario: func(c *check.C) {
				cronParent, err := s.client.BatchV1beta1().CronJobs("default").Get(context.TODO(), "myjob", v1.GetOptions{})
				c.Assert(err, check.IsNil)
				expected := &batchv1.Job{
					ObjectMeta: v1.ObjectMeta{
						Name:      "myjob-manual-trigger",
						Namespace: "default",
						Labels: map[string]string{
							"job.kubernetes.io/component":  "tsuru-job",
							"job.kubernetes.io/managed-by": "tsuru",
							"job.kubernetes.io/name":       "myjob",
							"tsuru.io/is-tsuru":            "true",
							"tsuru.io/is-service":          "true",
							"tsuru.io/job-name":            "myjob",
							"tsuru.io/job-pool":            "test-default",
							"tsuru.io/job-team":            "admin",
							"tsuru.io/is-job":              "true",
						},
						Annotations: map[string]string{"cronjob.kubernetes.io/instantiate": "manual"},
						OwnerReferences: []v1.OwnerReference{
							{
								Name:       cronParent.Name,
								Kind:       "CronJob",
								UID:        cronParent.UID,
								APIVersion: "batch/v1",
							},
						},
					},
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: v1.ObjectMeta{
								Labels: map[string]string{
									"job.kubernetes.io/component":  "tsuru-job",
									"job.kubernetes.io/managed-by": "tsuru",
									"job.kubernetes.io/name":       "myjob",
									"tsuru.io/is-tsuru":            "true",
									"tsuru.io/is-service":          "true",
									"tsuru.io/job-name":            "myjob",
									"tsuru.io/job-pool":            "test-default",
									"tsuru.io/job-team":            "admin",
									"tsuru.io/is-job":              "true",
								},
								Annotations: make(map[string]string),
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:    "c1",
										Image:   "ubuntu:latest",
										Command: []string{"echo", "hello world"},
									},
								},
								RestartPolicy: "OnFailure",
							},
						},
					},
				}
				listJobs, err := s.client.BatchV1().Jobs(expected.Namespace).List(context.TODO(), v1.ListOptions{})
				c.Assert(err, check.IsNil)
				c.Assert(len(listJobs.Items), check.Equals, 1)
				gotJob, err := s.client.BatchV1().Jobs(expected.Namespace).Get(context.TODO(), expected.Name, v1.GetOptions{})
				c.Assert(err, check.IsNil)
				c.Assert(gotJob, check.DeepEquals, expected)
				// cleanup
				err = s.client.BatchV1beta1().CronJobs(expected.Namespace).Delete(context.TODO(), "myjob", v1.DeleteOptions{})
				c.Assert(err, check.IsNil)
			},
		},
	}
	for _, tt := range tests {
		tt.setup()
		tt.scenario()
		tt.testScenario(c)
	}
}
