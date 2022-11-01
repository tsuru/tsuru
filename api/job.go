// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	stdContext "context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	pkgErrors "github.com/pkg/errors"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/job"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	appTypes "github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
)

type inputJob struct {
	TeamOwner   string            `json:"team-owner"`
	Plan        string            `json:"plan"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Pool        string            `json:"pool"`
	Metadata    appTypes.Metadata `json:"metadata"`

	Schedule   string                   `json:"schedule"`
	Containers []jobTypes.ContainerInfo `json:"containers"`
}

func getJob(ctx stdContext.Context, name, teamOwner string) (*job.Job, error) {
	j, err := job.GetByNameAndTeam(ctx, name, teamOwner)
	if err != nil {
		if err == jobTypes.ErrJobNotFound {
			return nil, &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("Job %s not found.", name)}
		}
		return nil, err
	}
	return j, nil
}

// title: job create
// path: /jobs
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//
//	201: Job created
//	400: Invalid data
//	401: Unauthorized
//	403: Quota exceeded
//	409: Job already exists
func createJob(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var ij inputJob
	err = ParseInput(r, &ij)
	if err != nil {
		return err
	}
	j := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner:   ij.TeamOwner,
			Plan:        appTypes.Plan{Name: ij.Plan},
			Name:        ij.Name,
			Description: ij.Description,
			Pool:        ij.Pool,
			Metadata:    ij.Metadata,
		},
		Schedule:   ij.Schedule,
		Containers: ij.Containers,
	}
	if j.TeamOwner == "" {
		j.TeamOwner, err = autoTeamOwner(ctx, t, permission.PermAppCreate)
		if err != nil {
			return err
		}
	}
	canCreate := permission.Check(t, permission.PermAppCreate,
		permission.Context(permTypes.CtxTeam, j.TeamOwner),
	)
	if !canCreate {
		return permission.ErrUnauthorized
	}
	u, err := auth.ConvertNewUser(t.User())
	if err != nil {
		return err
	}
	evt, err := event.New(&event.Opts{
		Target:     jobTarget(j.Name),
		Kind:       permission.PermAppCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForJob(&j)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = job.CreateJob(ctx, &j, u)
	if err != nil {
		log.Errorf("Got error while creating job: %s", err)
		if _, ok := err.(appTypes.NoTeamsError); ok {
			return &errors.HTTP{
				Code:    http.StatusBadRequest,
				Message: "In order to create a job, you should be member of at least one team",
			}
		}
		if e, ok := err.(*jobTypes.JobCreationError); ok {
			if e.Err == jobTypes.ErrJobAlreadyExists {
				return &errors.HTTP{Code: http.StatusConflict, Message: e.Error()}
			}
			if _, ok := pkgErrors.Cause(e.Err).(*quota.QuotaExceededError); ok {
				return &errors.HTTP{
					Code:    http.StatusForbidden,
					Message: "Quota exceeded",
				}
			}
		}
		return err
	}
	msg := map[string]interface{}{
		"status": "success",
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(jsonMsg)
	return nil
}

// title: remove job
// path: /jobs/{name}
// method: DELETE
// produce: application/x-json-stream
// responses:
//
//	200: Job removed
//	401: Unauthorized
//	404: Not found
func deleteJob(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var ij inputJob
	err = ParseInput(r, &ij)
	if err != nil {
		return err
	}

	fmt.Printf("%+v\n", ij)

	j, err := getJob(ctx, ij.Name, ij.TeamOwner)
	if err != nil {
		return err
	}
	canDelete := permission.Check(t, permission.PermAppDelete,
		contextsForJob(j)...,
	)
	if !canDelete {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     jobTarget(j.Name),
		Kind:       permission.PermAppDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForJob(j)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt.SetLogWriter(writer)
	w.Header().Set("Content-Type", "application/x-json-stream")
	if err = job.RemoveJobFromDb(j.Name, j.TeamOwner); err != nil {
		return err
	}
	return job.DeleteFromProvisioner(ctx, j)
}

func jobTarget(jobName string) event.Target {
	return event.Target{Type: event.TargetTypeJob, Value: jobName}
}

func contextsForJob(job *job.Job) []permTypes.PermissionContext {
	return append(permission.Contexts(permTypes.CtxTeam, job.Teams),
		permission.Context(permTypes.CtxApp, job.Name),
		permission.Context(permTypes.CtxPool, job.Pool),
	)
}
