// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/job"
	jobTypes "github.com/tsuru/tsuru/types/job"
	check "gopkg.in/check.v1"
	apiv1beta1 "k8s.io/api/batch/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) TestProvisionerCreateJob(c *check.C) {
	waitCron := s.mock.CronJobReactions(c)
	defer waitCron()

	tests := []struct {
		scenario       func()
		expectedTarget apiv1beta1.CronJob
	}{
		{
			scenario: func() {
				cj := job.Job{
					TsuruJob: job.TsuruJob{
						Name:      "myjob",
						TeamOwner: s.team.Name,
						Pool:      "test-default",
					},
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						Name:    "c1",
						Image:   "ubuntu:latest",
						Command: []string{"echo", "hello world"},
					},
				}
				_, err := s.p.CreateJob(context.TODO(), &cj)
				waitCron()
				c.Assert(err, check.IsNil)
			},
			expectedTarget: apiv1beta1.CronJob{
				ObjectMeta: v1.ObjectMeta{
					Name: "myjob",
				},
			},
		},
	}

	for _, tt := range tests {
		tt.scenario()
		_, _ = s.client.BatchV1beta1().CronJobs("test-default").Get(context.TODO(), "myjob", v1.GetOptions{})
		// c.Assert(err, check.IsNil)
		// c.Assert(tt.expectedTarget.Name, check.Equals, cron.Name)
	}

}
