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
	jobTypes "github.com/tsuru/tsuru/types/job"
)

func checkCollision(ctx context.Context, jobName, teamOwner string) bool {
	_, err := GetByNameAndTeam(ctx, jobName, teamOwner)
	return err != jobTypes.ErrJobNotFound
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
	for i := 0; i < jobTypes.MaxAttempts; i++ {
		if collision = checkCollision(ctx, job.Name, job.TeamOwner); !collision {
			break
		}
	}
	if collision {
		return jobTypes.ErrMaxAttemptsReached
	}
	return nil
}

func buildName(ctx context.Context, job *Job) error {
	if job.IsCron() {
		if _, err := GetByNameAndTeam(ctx, job.Name, job.TeamOwner); err != nil && err != jobTypes.ErrJobNotFound {
			return errors.WithMessage(err, "unable to check if job already exists")
		}
	} else {
		// If it's a one-time-job we must generate a unique job name to save in the database
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
	}
	if err != nil {
		return err
	}
	job.Plan = *plan
	return nil
}

func buildTsuruInfo(ctx context.Context, job *Job, user *auth.User) {
	job.Teams = []string{job.TeamOwner}
	job.Owner = user.Email
	// if !job.IsCron() {
	// 	job.CreatedAt = &time.Time{}
	// 	*job.CreatedAt = time.Now()
	// }
}
