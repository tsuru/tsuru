// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrJobAlreadyExists = errors.New("there is already a job with this name")
)

// insertJob is an action that inserts a job in the database in Forward and
// removes it in the Backward.
//
// The first argument in the context must be a Job or a pointer to a Job.
var insertJob = action.Action{
	Name: "insert-job",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var job *Job
		switch ctx.Params[0].(type) {
		case *Job:
			job = ctx.Params[0].(*Job)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		err := insertJobDB(job)
		if err != nil {
			return nil, err
		}
		return job, nil
	},
	Backward: func(ctx action.BWContext) {
		job := ctx.FWResult.(*Job)
		removeJobDB(job)
	},
	MinParams: 1,
}

func insertJobDB(job *Job) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Jobs().Insert(job)
	if mgo.IsDup(err) {
		return ErrJobAlreadyExists
	}
	return nil
}

func removeJobDB(job *Job) error {
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Could not connect to the database: %s", err)
		return err
	}
	defer conn.Close()
	conn.Jobs().Remove(bson.M{"name": job.Name})
	return nil
}

var reserveTeamCronjob = action.Action{
	Name: "reserve-team-job",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var job *Job
		switch ctx.Params[0].(type) {
		case *Job:
			job = ctx.Params[0].(*Job)
			if !job.IsCron {
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
			if !job.IsCron {
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
