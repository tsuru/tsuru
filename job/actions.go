// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
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
	err = conn.Apps().Insert(job)
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
