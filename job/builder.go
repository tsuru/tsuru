// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"
	"regexp"

	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	jobTypes "github.com/tsuru/tsuru/types/job"
)

var jobNameRegexp = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

func validateName(ctx context.Context, job *jobTypes.Job) error {
	if !jobNameRegexp.MatchString(job.Name) {
		return jobTypes.ErrInvalidJobName
	}
	// check if the given name is already in the database
	if _, err := servicemanager.Job.GetByName(ctx, job.Name); err == nil {
		return jobTypes.ErrJobAlreadyExists
	}
	return nil
}

func buildPlan(ctx context.Context, job *jobTypes.Job) error {
	jobPool, err := pool.GetPoolByName(ctx, job.Pool)
	if err != nil {
		return err
	}
	plan, err := jobPool.GetDefaultPlan(ctx)
	if err != nil {
		return err
	}
	if job.Plan.Name != "" {
		plan, err = servicemanager.Plan.FindByName(ctx, job.Plan.Name)
		if err != nil {
			return err
		}
	}
	job.Plan = *plan
	return nil
}

func buildTsuruInfo(job *jobTypes.Job, user *authTypes.User) {
	job.Owner = user.Email
}

func buildFakeSchedule(job *jobTypes.Job) {
	// trick based on fact that crontab syntax is not strictly validated
	job.Spec.Schedule = "* * 31 2 *"
}

func buildActiveDeadline(activeDeadlineSeconds *int64) *int64 {
	defaultActiveDeadline := func() *int64 { i := int64(0); return &i }()
	if activeDeadlineSeconds != nil && *activeDeadlineSeconds == int64(0) {
		return defaultActiveDeadline
	}
	return activeDeadlineSeconds
}
