// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	jobTypes "github.com/tsuru/tsuru/types/job"
	"gopkg.in/mgo.v2/bson"
)

var provisionJob = action.Action{
	Name: "provision-job",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var job *Job
		switch ctx.Params[0].(type) {
		case *Job:
			job = ctx.Params[0].(*Job)
		default:
			return nil, errors.New("First parameter must be *Job.")
		}
		prov, err := job.getProvisioner()
		if err != nil {
			return nil, err
		}
		return prov.CreateJob(ctx.Context, job)
	},
	Backward: func(ctx action.BWContext) {
		var job *Job
		switch ctx.Params[0].(type) {
		case *Job:
			job = ctx.Params[0].(*Job)
		default:
			return
		}
		prov, err := job.getProvisioner()
		if err == nil {
			prov.DestroyJob(ctx.Context, job)
		}
	},
	MinParams: 1,
}

var updateJobProv = action.Action{
	Name: "update-job",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var job *Job
		switch ctx.Params[0].(type) {
		case *Job:
			job = ctx.Params[0].(*Job)
		default:
			return nil, errors.New("First parameter must be *Job.")
		}
		prov, err := job.getProvisioner()
		if err != nil {
			return nil, err
		}
		return nil, prov.UpdateJob(ctx.Context, job)
	},
	MinParams: 1,
}

// updateJob is an action that updates a job in the database in Forward and
// does nothing in the Backward.
//
// The first argument in the context must be a Job or a pointer to a Job.
var jobUpdateDB = action.Action{
	Name: "update-job-db",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var j *Job
		switch ctx.Params[0].(type) {
		case *Job:
			j = ctx.Params[0].(*Job)
		default:
			return nil, errors.New("First parameter must be *Job.")
		}
		return nil, updateJobDB(j)
	},
	MinParams: 1,
}

// insertJob is an action that inserts a job in the database in Forward and
// removes it in the Backward.
// insert job must always be run after provision-job because it depends on
// the value of ctx.Previous
//
// The first argument in the context must be a Job or a pointer to a Job.
var insertJob = action.Action{
	Name: "insert-job",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var j *Job
		switch ctx.Params[0].(type) {
		case *Job:
			j = ctx.Params[0].(*Job)
		default:
			return nil, errors.New("First parameter must be *Job.")
		}
		var err error
		j.Name = ctx.Previous.(string)
		if err != nil {
			return nil, err
		}
		err = insertJobDB(j)
		if err != nil {
			return nil, err
		}
		return j, nil
	},
	Backward: func(ctx action.BWContext) {
		job := ctx.FWResult.(*Job)
		RemoveJobFromDb(job.Name, job.TeamOwner)
	},
	MinParams: 1,
}

func insertJobDB(job *Job) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = GetByNameAndTeam(job.ctx, job.Name, job.TeamOwner)
	if err == jobTypes.ErrJobNotFound {
		return conn.Jobs().Insert(job)
	} else if err == nil {
		return jobTypes.ErrJobAlreadyExists
	}
	return err
}

func updateJobDB(job *Job) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	oldJob, err := GetByNameAndTeam(job.ctx, job.Name, job.TeamOwner)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(*oldJob, *job) {
		return errors.New(fmt.Sprintf("no new values to be patched into job %s", job.Name))
	}
	return conn.Jobs().Update(bson.M{"tsurujob.name": job.Name, "tsurujob.teamowner": job.TeamOwner}, job)
}

var reserveTeamCronjob = action.Action{
	Name: "reserve-team-job",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var job *Job
		switch ctx.Params[0].(type) {
		case *Job:
			job = ctx.Params[0].(*Job)
			if !job.IsCron() {
				return nil, errors.New("job type must be cron to increment team quota")
			}
		default:
			return nil, errors.New("first parameter must be *Job.")
		}
		if err := servicemanager.TeamQuota.Inc(ctx.Context, &authTypes.Team{Name: job.TeamOwner}, 1); err != nil {
			return nil, err
		}
		return map[string]string{"job": job.Name, "team": job.TeamOwner}, nil
	},
	Backward: func(ctx action.BWContext) {
		m := ctx.FWResult.(map[string]string)
		if teamStr, ok := m["team"]; ok {
			servicemanager.TeamQuota.Inc(ctx.Context, &authTypes.Team{Name: teamStr}, -1)
		}
	},
	MinParams: 2,
}

// reserveUserCronjob reserves the job for the user, only if the user has a quota
// of jobs. If the user does not have a quota, meaning that it's unlimited,
// reserveUserCronjob.Forward just return nil.
var reserveUserCronjob = action.Action{
	Name: "reserve-user-cronjob",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var job *Job
		switch ctx.Params[0].(type) {
		case *Job:
			job = ctx.Params[0].(*Job)
			if !job.IsCron() {
				return nil, errors.New("job type must be cron to increment team quota")
			}
		default:
			return nil, errors.New("First parameter must be *Job.")
		}
		var user auth.User
		switch ctx.Params[1].(type) {
		case auth.User:
			user = ctx.Params[1].(auth.User)
		case *auth.User:
			user = *ctx.Params[1].(*auth.User)
		default:
			return nil, errors.New("Second parameter must be auth.User or *auth.User.")
		}
		if user.FromToken {
			// there's no quota to update as the user was generated from team token.
			return map[string]string{"job": job.Name}, nil
		}
		u := auth.User(user)
		if err := servicemanager.UserQuota.Inc(ctx.Context, &u, 1); err != nil {
			return nil, err
		}
		return map[string]string{"job": job.Name, "user": user.Email}, nil
	},
	Backward: func(ctx action.BWContext) {
		m, found := ctx.FWResult.(map[string]string)
		if !found {
			return
		}
		email, found := m["user"]
		if !found {
			return
		}
		if user, err := auth.GetUserByEmail(email); err == nil {
			servicemanager.UserQuota.Inc(ctx.Context, user, -1)
		}
	},
	MinParams: 2,
}
