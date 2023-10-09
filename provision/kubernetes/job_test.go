// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/types/app"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"
)

func (s *S) TestProvisionerCreateCronJob(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()
	tests := []struct {
		name      string
		scenario  func()
		namespace string
		jobName   string
		assertion func(c *check.C, gotCron *batchv1.CronJob)
		teardown  func()
	}{
		{
			name:      "simple create cronjob with default plan",
			jobName:   "myjob",
			namespace: "default",
			teardown: func() {
				j := jobTypes.Job{
					Name: "myjob",
					Pool: "test-default",
				}
				err := s.p.DestroyJob(context.TODO(), &j)
				c.Assert(err, check.IsNil)
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
							Image:   "ubuntu:latest",
							Command: []string{"echo", "hello world"},
						},
					},
				}
				_, err := s.p.CreateJob(context.TODO(), &cj)
				waitCron()
				c.Assert(err, check.IsNil)
			},
			assertion: func(c *check.C, gotCron *batchv1.CronJob) {
				expectedTarget := &batchv1.CronJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "myjob",
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
							"tsuru.io/is-deploy":           "false",
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
									ObjectMeta: v1.ObjectMeta{
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
											"tsuru.io/is-deploy":           "false",
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
												Env:     []corev1.EnvVar{},
												Resources: apiv1.ResourceRequirements{
													Limits: corev1.ResourceList{
														apiv1.ResourceEphemeralStorage: resource.MustParse("100Mi"),
													},
													Requests: corev1.ResourceList{
														apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
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
				c.Assert(*gotCron, check.DeepEquals, *expectedTarget)
				account, err := s.client.CoreV1().ServiceAccounts(expectedTarget.Namespace).Get(context.TODO(), "job-"+expectedTarget.Name, metav1.GetOptions{})
				c.Assert(err, check.IsNil)
				c.Assert(account, check.DeepEquals, &apiv1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job-" + expectedTarget.Name,
						Namespace: expectedTarget.Namespace,
						Labels: map[string]string{
							"tsuru.io/is-tsuru":    "true",
							"tsuru.io/job-name":    expectedTarget.Name,
							"tsuru.io/provisioner": "kubernetes",
						},
					},
				})
			},
		},
		{
			name:      "create cronjob with service account annotation",
			jobName:   "myjob",
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
							Image:   "ubuntu:latest",
							Command: []string{"echo", "hello world"},
						},
					},
				}
				_, err := s.p.CreateJob(context.TODO(), &cj)
				waitCron()
				c.Assert(err, check.IsNil)
			},
			assertion: func(c *check.C, gotCron *batchv1.CronJob) {
				account, err := s.client.CoreV1().ServiceAccounts(gotCron.Namespace).Get(context.TODO(), "job-"+gotCron.Name, metav1.GetOptions{})
				c.Assert(err, check.IsNil)
				c.Assert(account, check.DeepEquals, &apiv1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job-" + gotCron.Name,
						Namespace: gotCron.Namespace,
						Labels: map[string]string{
							"tsuru.io/is-tsuru":    "true",
							"tsuru.io/job-name":    gotCron.Name,
							"tsuru.io/provisioner": "kubernetes",
						},
						Annotations: map[string]string{
							"iam.gke.io/gcp-service-account": "test@test.com",
						},
					},
				})
			},
			teardown: func() {
				err := s.p.DestroyJob(context.TODO(), &jobTypes.Job{
					Name: "myjob",
					Pool: "test-default",
				})
				c.Assert(err, check.IsNil)
			},
		},
	}
	for _, tt := range tests {
		tt.scenario()
		gotCron, err := s.client.BatchV1().CronJobs(tt.namespace).Get(context.TODO(), tt.jobName, v1.GetOptions{})
		c.Assert(err, check.IsNil)
		tt.assertion(c, gotCron)
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
						Parallelism:           func() *int32 { r := int32(2); return &r }(),
						Completions:           func() *int32 { r := int32(1); return &r }(),
						ActiveDeadlineSeconds: func() *int64 { r := int64(0); return &r }(),
						BackoffLimit:          func() *int32 { r := int32(6); return &r }(),
						Container: jobTypes.ContainerInfo{
							Image:   "ubuntu:latest",
							Command: []string{"echo", "hello world"},
						},
					},
				}
				err := s.p.UpdateJob(context.TODO(), &newCJ)
				waitCron()
				c.Assert(err, check.IsNil)
			},
			expectedTarget: &batchv1.CronJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "myjob",
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
						"tsuru.io/is-deploy":           "false",
						"label2":                       "value3",
					},
					Annotations: map[string]string{"annotation2": "value4"},
				},
				Spec: batchv1.CronJobSpec{
					Schedule: "* * * * *",
					Suspend:  func() *bool { r := false; return &r }(),
					JobTemplate: batchv1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							TTLSecondsAfterFinished: func() *int32 { defaultTTL := int32(86400); return &defaultTTL }(),
							Parallelism:             func() *int32 { r := int32(2); return &r }(),
							Completions:             func() *int32 { r := int32(1); return &r }(),
							ActiveDeadlineSeconds:   func() *int64 { r := int64(60 * 60); return &r }(),
							BackoffLimit:            func() *int32 { r := int32(6); return &r }(),
							Template: corev1.PodTemplateSpec{
								ObjectMeta: v1.ObjectMeta{
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
										"tsuru.io/is-deploy":           "false",
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
											Resources: apiv1.ResourceRequirements{
												Limits: corev1.ResourceList{
													apiv1.ResourceEphemeralStorage: resource.MustParse("100Mi"),
												},
												Requests: corev1.ResourceList{
													apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
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
		gotCron, err := s.client.BatchV1().CronJobs(tt.expectedTarget.Namespace).Get(context.TODO(), tt.expectedTarget.Name, v1.GetOptions{})
		c.Assert(err, check.IsNil)
		c.Assert(*gotCron, check.DeepEquals, *tt.expectedTarget)
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
				_, err := s.client.BatchV1().CronJobs("default").Get(context.TODO(), "mycronjob", v1.GetOptions{})
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
				cj := jobTypes.Job{
					Name:      "myjob",
					TeamOwner: s.team.Name,
					Pool:      "pool1",
					Spec: jobTypes.JobSpec{
						Schedule: "* * * * *",
						Container: jobTypes.ContainerInfo{
							Image:   "ubuntu:latest",
							Command: []string{"echo", "hello world"},
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
				cronParent, err := s.client.BatchV1().CronJobs("default").Get(context.TODO(), "myjob", v1.GetOptions{})
				c.Assert(err, check.IsNil)
				expected := &batchv1.Job{
					ObjectMeta: v1.ObjectMeta{
						Name:      "myjob-manual-trigger",
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
							"tsuru.io/is-deploy":           "false",
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
						TTLSecondsAfterFinished: func() *int32 { defaultTTL := int32(86400); return &defaultTTL }(),
						ActiveDeadlineSeconds:   func() *int64 { r := int64(60 * 60); return &r }(),
						Template: corev1.PodTemplateSpec{
							ObjectMeta: v1.ObjectMeta{
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
									"tsuru.io/is-deploy":           "false",
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
												Name:  "MY_ENV",
												Value: "** value",
											},
											{
												Name:  "REDIS_HOST",
												Value: "localhost",
											},
										},
										Resources: apiv1.ResourceRequirements{
											Limits: corev1.ResourceList{
												apiv1.ResourceEphemeralStorage: resource.MustParse("100Mi"),
											},
											Requests: corev1.ResourceList{
												apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
											},
										},
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
				err = s.client.BatchV1().CronJobs(expected.Namespace).Delete(context.TODO(), "myjob", v1.DeleteOptions{})
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

func (s *S) TestCreateJobEvent(c *check.C) {
	boolTrue := true
	cleanup := func() {
		err := dbtest.ClearAllCollections(s.conn.Apps().Database)
		c.Assert(err, check.IsNil)
	}
	tests := []struct {
		name         string
		scenario     func()
		testScenario func(c *check.C)
	}{
		{
			name: "when job and evt (jobcreate) are valid",
			scenario: func() {
				j := &batchv1.Job{
					ObjectMeta: v1.ObjectMeta{
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
							ObjectMeta: v1.ObjectMeta{
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
				evt := &apiv1.Event{
					ObjectMeta: v1.ObjectMeta{
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
					Message: "Created pod: myjob-somehash",
					Reason:  "SuccessfulCreate",
					Type:    "Normal",
				}
				createJobEvent(j, evt)
			},
			testScenario: func(c *check.C) {
				evts, err := event.List(&event.Filter{})
				c.Assert(err, check.IsNil)
				c.Assert(len(evts), check.Equals, 1)
				gotEvt := evts[0]
				c.Assert(gotEvt, check.NotNil)
				c.Assert(gotEvt.Target, check.DeepEquals, event.Target{Type: event.TargetTypeJob, Value: "myjob"})
				c.Assert(gotEvt.Allowed, check.DeepEquals, event.Allowed(permission.PermJobReadEvents, permission.Context(permTypes.CtxJob, "myjob")))
				c.Assert(gotEvt.Owner.Type, check.DeepEquals, event.OwnerTypeInternal)
				c.Assert(gotEvt.Cancelable, check.Equals, false)
				expectedCustomData := map[string]string{
					"job-name":           "myjob",
					"job-controller":     "myjob",
					"event-type":         "Normal",
					"event-reason":       "SuccessfulCreate",
					"message":            "Created pod: myjob-somehash",
					"cluster-start-time": v1.Time{}.String(),
				}
				gotCustomData := map[string]string{}
				err = gotEvt.EndCustomData.Unmarshal(&gotCustomData)
				c.Assert(err, check.IsNil)
				c.Assert(gotCustomData, check.DeepEquals, expectedCustomData)
				c.Assert(gotEvt.Error, check.Equals, "")
			},
		},
		{
			name: "when job and evt (jobrun) are valid and job has cronjob owner reference",
			scenario: func() {
				j := &batchv1.Job{
					ObjectMeta: v1.ObjectMeta{
						Name:      "myjob",
						Namespace: "default",
						Labels: map[string]string{
							"tsuru.io/is-tsuru": "true",
							"tsuru.io/job-name": "myjob",
							"tsuru.io/job-pool": "test-default",
							"tsuru.io/job-team": "admin",
							"tsuru.io/is-job":   "true",
						},
						OwnerReferences: []v1.OwnerReference{
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
							ObjectMeta: v1.ObjectMeta{
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
				evt := &apiv1.Event{
					ObjectMeta: v1.ObjectMeta{
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
				createJobEvent(j, evt)
			},
			testScenario: func(c *check.C) {
				evts, err := event.List(&event.Filter{})
				c.Assert(err, check.IsNil)
				c.Assert(len(evts), check.Equals, 1)
				gotEvt := evts[0]
				c.Assert(gotEvt, check.NotNil)
				c.Assert(gotEvt.Target, check.DeepEquals, event.Target{Type: event.TargetTypeJob, Value: "cronjob-parent"})
				c.Assert(gotEvt.Allowed, check.DeepEquals, event.Allowed(permission.PermJobReadEvents, permission.Context(permTypes.CtxJob, "cronjob-parent")))
				c.Assert(gotEvt.Owner.Type, check.DeepEquals, event.OwnerTypeInternal)
				c.Assert(gotEvt.Cancelable, check.Equals, false)
				expectedCustomData := map[string]string{
					"job-name":           "myjob",
					"job-controller":     "cronjob-parent",
					"event-type":         "Normal",
					"event-reason":       "Completed",
					"message":            "Job completed",
					"cluster-start-time": v1.Time{}.String(),
				}
				gotCustomData := map[string]string{}
				err = gotEvt.EndCustomData.Unmarshal(&gotCustomData)
				c.Assert(err, check.IsNil)
				c.Assert(gotCustomData, check.DeepEquals, expectedCustomData)
				c.Assert(gotEvt.Error, check.Equals, "")
			},
		},
		{
			name: "when job and evt (backofflimitexceeded) are valid",
			scenario: func() {
				j := &batchv1.Job{
					ObjectMeta: v1.ObjectMeta{
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
							ObjectMeta: v1.ObjectMeta{
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
				evt := &apiv1.Event{
					ObjectMeta: v1.ObjectMeta{
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
				createJobEvent(j, evt)
			},
			testScenario: func(c *check.C) {
				evts, err := event.List(&event.Filter{})
				c.Assert(err, check.IsNil)
				c.Assert(len(evts), check.Equals, 1)
				gotEvt := evts[0]
				c.Assert(gotEvt, check.NotNil)
				c.Assert(gotEvt.Target, check.DeepEquals, event.Target{Type: event.TargetTypeJob, Value: "myjob"})
				c.Assert(gotEvt.Allowed, check.DeepEquals, event.Allowed(permission.PermJobReadEvents, permission.Context(permTypes.CtxJob, "myjob")))
				c.Assert(gotEvt.Owner.Type, check.DeepEquals, event.OwnerTypeInternal)
				c.Assert(gotEvt.Cancelable, check.Equals, false)
				expectedCustomData := map[string]string{
					"job-name":           "myjob",
					"job-controller":     "myjob",
					"event-type":         "Warning",
					"event-reason":       "BackoffLimitExceeded",
					"message":            "Job has reached the specified backoff limit",
					"cluster-start-time": v1.Time{}.String(),
				}
				gotCustomData := map[string]string{}
				err = gotEvt.EndCustomData.Unmarshal(&gotCustomData)
				c.Assert(err, check.IsNil)
				c.Assert(gotCustomData, check.DeepEquals, expectedCustomData)
			},
		},
		{
			name: "when evt reason does not apply",
			scenario: func() {
				j := &batchv1.Job{
					ObjectMeta: v1.ObjectMeta{
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
							ObjectMeta: v1.ObjectMeta{
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
				evt := &apiv1.Event{
					ObjectMeta: v1.ObjectMeta{
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
				createJobEvent(j, evt)
			},
			testScenario: func(c *check.C) {
				evts, err := event.List(&event.Filter{})
				c.Assert(err, check.IsNil)
				c.Assert(len(evts), check.Equals, 0)
			},
		},
	}

	for _, tt := range tests {
		tt.scenario()
		tt.testScenario(c)
		cleanup()
	}
}
