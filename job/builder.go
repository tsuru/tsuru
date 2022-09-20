// App is the main type in tsuru. An app represents a real world application.
// This struct holds information about the app: its name, address, list of
// teams that have access to it, used platform, etc.

package job

import (
	"context"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
)

func checkCollision(ctx context.Context, jobName string) bool {
	_, err := GetJobByName(ctx, jobName)
	if err == ErrJobNotFound {
		return false
	}
	return true
}

func (job *Job) genUniqueName() error {
	id, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	job.Name = id.String()
	return nil
}

func oneTimeJobName(ctx context.Context, job *Job) error {
	job.genUniqueName()
	collision := true
	for i := 0; i < maxAttempts; i++ {
		if collision = checkCollision(ctx, job.Name); collision == false {
			break
		}
	}
	if collision == true {
		return ErrMaxAttemptsReached
	}
	return nil
}

func buildName(ctx context.Context, job *Job) error {
	// If it's a one-time-job we must generate a unique job name to save in the database
	if job.IsCron {
		if _, err := GetJobByName(ctx, job.Name); err != nil && err != ErrJobNotFound {
			return errors.WithMessage(err, "unable to check if job already exists")
		}
	} else {
		oneTimeJobName(ctx, job)
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
	}
	if err != nil {
		return err
	}
	job.Plan = *plan
	return nil
}

func buildOwnerInfo(ctx context.Context, job *Job, user *auth.User) {
	job.Teams = []string{job.TeamOwner}
	job.Owner = user.Email
}
