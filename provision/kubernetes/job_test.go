// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/job"
	jobTypes "github.com/tsuru/tsuru/types/job"
	check "gopkg.in/check.v1"
	batchv1 "k8s.io/api/batch/v1"
	apiv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) TestProvisionerCreateJob(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	tests := []struct {
		scenario       func()
		expectedTarget *apiv1beta1.CronJob
	}{
		{
			scenario: func() {
				cj := job.Job{
						Name:      "myjob",
						TeamOwner: s.team.Name,
						Pool:      "test-default",
						Spec: job.JobSpec{
							Schedule: "* * * * *",
							Parallelism: func() *int32 { r := int32(3); return &r}(),
							Completions: func() *int32 { r := int32(1); return &r}(),
							ActiveDeadlineSeconds: func() *int64 { r := int64(5*60); return &r}(),
							BackoffLimit: func() *int32 { r := int32(7); return &r}(),
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
					Name: "myjob",
					Namespace: "default",
					Labels: map[string]string{
					"job.kubernetes.io/component":"tsuru-job",
					"job.kubernetes.io/managed-by":"tsuru",
					"job.kubernetes.io/name":"myjob",
					"tsuru.io/is-tsuru":"true",
					"tsuru.io/job-name":"myjob",
					"tsuru.io/job-pool":"test-default",
					"tsuru.io/job-team":"admin",
				},
				Annotations: make(map[string]string),
			},
			Spec: apiv1beta1.CronJobSpec{
				Schedule: "* * * * *",
				JobTemplate: apiv1beta1.JobTemplateSpec{
					Spec: batchv1.JobSpec{
						Parallelism: func() *int32 { r := int32(3); return &r}(),
						Completions: func() *int32 { r := int32(1); return &r}(),
						ActiveDeadlineSeconds: func() *int64 { r := int64(5*60); return &r}(),
						BackoffLimit: func() *int32 { r := int32(7); return &r}(),
						Template: corev1.PodTemplateSpec{
							ObjectMeta: v1.ObjectMeta{
								Labels: map[string]string{
									"job.kubernetes.io/component":"tsuru-job",
									"job.kubernetes.io/managed-by":"tsuru",
									"job.kubernetes.io/name":"myjob",
									"tsuru.io/is-tsuru":"true",
									"tsuru.io/job-name":"myjob",
									"tsuru.io/job-pool":"test-default",
									"tsuru.io/job-team":"admin",
								},
								Annotations: make(map[string]string),
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "c1",
										Image: "ubuntu:latest",
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
		gotCron, err := s.client.BatchV1beta1().CronJobs("default").Get(context.TODO(), "myjob", v1.GetOptions{})
		c.Assert(err, check.IsNil)
		c.Assert(*gotCron, check.DeepEquals, *tt.expectedTarget)
	}
}
