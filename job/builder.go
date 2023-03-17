// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	jobTypes "github.com/tsuru/tsuru/types/job"
)

func checkCollision(ctx context.Context, jobName string) bool {
	_, err := GetByName(ctx, jobName)
	return err != jobTypes.ErrJobNotFound
}

func (job *Job) genUniqueName() error {
	id, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	job.Name = fmt.Sprintf("tsuru-%s", id.String())
	return nil
}

func oneTimeJobName(ctx context.Context, job *Job) error {
	collision := true
	for i := 0; i < jobTypes.MaxAttempts; i++ {
		if err := job.genUniqueName(); err != nil {
			return err
		}
		if collision = checkCollision(ctx, job.Name); !collision {
			break
		}
	}
	if collision {
		return jobTypes.ErrMaxAttemptsReached
	}
	return nil
}

func buildName(ctx context.Context, job *Job) error {
	if job.Name != "" {
		// check if the given name is already in the database
		if _, err := GetByName(ctx, job.Name); err == nil {
			return jobTypes.ErrJobAlreadyExists
		}
	} else {
		if job.IsCron() {
			return errors.New("cronjob name can't be empty")
		}
		// If it's a one-time-job a unique job name is provided
		return oneTimeJobName(ctx, job)
	}
	return nil
}

func buildPlan(ctx context.Context, job *Job) error {
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

func buildTsuruInfo(ctx context.Context, job *Job, user *auth.User) {
	job.Teams = []string{job.TeamOwner}
	job.Owner = user.Email
}
