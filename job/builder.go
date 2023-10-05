// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	jobTypes "github.com/tsuru/tsuru/types/job"
)

func validateName(ctx context.Context, job *jobTypes.Job) error {
	if job.Name == "" {
		return errors.New("cronjob name can't be empty")
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
	plan, err := jobPool.GetDefaultPlan()
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

func buildTsuruInfo(ctx context.Context, job *jobTypes.Job, user *authTypes.User) {
	job.Teams = []string{job.TeamOwner}
	job.Owner = user.Email
}

func buildFakeSchedule(ctx context.Context, job *jobTypes.Job) {
	// trick based on fact that crontab syntax is not strictly validated
	job.Spec.Schedule = "* * 31 2 *"
}

func buildActiveDeadline(activeDeadlineSeconds *int64) *int64 {
	defaultActiveDeadline := int64(60 * 60)
	if activeDeadlineSeconds == nil || *activeDeadlineSeconds == int64(0) {
		return &defaultActiveDeadline
	}
	return activeDeadlineSeconds
}
