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

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/job"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

type inputJob struct {
	TeamOwner   string            `json:"team-owner"`
	Plan        string            `json:"plan"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Pool        string            `json:"pool"`
	Metadata    appTypes.Metadata `json:"metadata"`

	Schedule  string                 `json:"schedule"`
	Container jobTypes.ContainerInfo `json:"container"`
	Trigger   bool                   `json:"trigger"` // Trigger means the client wants to forcefully run a job or a cronjob
}

func getJob(ctx stdContext.Context, name string) (*job.Job, error) {
	j, err := job.GetByName(ctx, name)
	if err != nil {
		if err == jobTypes.ErrJobNotFound {
			return nil, &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("Job %s not found.", name)}
		}
		return nil, err
	}
	return j, nil
}

func jobFilterByContext(contexts []permTypes.PermissionContext, filter *job.Filter) *job.Filter {
	if filter == nil {
		filter = &job.Filter{}
	}
contextsLoop:
	for _, c := range contexts {
		switch c.CtxType {
		case permTypes.CtxGlobal:
			filter.Extra = nil
			break contextsLoop
		case permTypes.CtxTeam:
			filter.ExtraIn("teams", c.Value)
		case permTypes.CtxJob:
			filter.ExtraIn("name", c.Value)
		case permTypes.CtxPool:
			filter.ExtraIn("pool", c.Value)
		}
	}
	return filter
}

// title: job list
// path: /jobs/list
// method: GET
// produce: application/json
// responses:
//
//	200: List jobs
//	204: No content
//	401: Unauthorized
func jobList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	filter := &job.Filter{}
	if name := r.URL.Query().Get("name"); name != "" {
		filter.Name = name
	}
	if teamOwner := r.URL.Query().Get("teamOwner"); teamOwner != "" {
		filter.TeamOwner = teamOwner
	}
	if owner := r.URL.Query().Get("owner"); owner != "" {
		filter.UserOwner = owner
	}
	if pool := r.URL.Query().Get("pool"); pool != "" {
		filter.Pool = pool
	}
	jobs, err := job.List(ctx, jobFilterByContext(permission.ContextsForPermission(t, permission.PermJobRead), filter))
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return json.NewEncoder(w).Encode(jobs)
}

// title: job trigger
// path: /job/trigger/{name}
// method: PUT
// produce: application/x-json-stream
// responses:
//
//	200: OK
//	401: Unauthorized
//	404: Not found
func jobTrigger(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	name := r.URL.Query().Get(":name")
	j, err := getJob(ctx, name)
	if err != nil {
		return err
	}
	canRun := permission.Check(t, permission.PermJobRun,
		contextsForJob(j)...,
	)
	if !canRun {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     jobTarget(j.Name),
		Kind:       permission.PermJobRun,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermJobReadEvents, contextsForJob(j)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = job.Trigger(ctx, j)
	if err != nil {
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
	w.WriteHeader(http.StatusOK)
	w.Write(jsonMsg)
	return nil
}

// title: job info
// path: /jobs
// method: GET
// produce: application/x-json-stream
// responses:
//
//	200: OK
//	401: Unauthorized
//	404: Not found
func jobInfo(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	name := r.URL.Query().Get(":name")
	j, err := getJob(ctx, name)
	if err != nil {
		return err
	}
	canGet := permission.Check(t, permission.PermJobRead,
		contextsForJob(j)...,
	)
	if !canGet {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     jobTarget(j.Name),
		Kind:       permission.PermJobRead,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermJobReadEvents, contextsForJob(j)...),
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
	units, err := j.Units()
	if err != nil {
		return err
	}
	result := struct {
		Job   job.Job          `json:"job,omitempty"`
		Units []provision.Unit `json:"units,omitempty"`
	}{
		Job:   *j,
		Units: units,
	}
	jsonMsg, err := json.Marshal(&result)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonMsg)
	return nil
}

// title: job update
// path: /jobs
// method: PUT
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//
//	201: Job updated
//	400: Invalid data
//	401: Unauthorized
func updateJob(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var ij inputJob
	err = ParseInput(r, &ij)
	if err != nil {
		return err
	}
	oldJob, err := getJob(ctx, ij.Name)
	if err != nil {
		return err
	}
	canUpdate := permission.Check(t, permission.PermAppUpdate,
		permission.Context(permTypes.CtxTeam, oldJob.TeamOwner),
	)
	if !canUpdate {
		return permission.ErrUnauthorized
	}
	u, err := auth.ConvertNewUser(t.User())
	if err != nil {
		return err
	}
	newJob := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner:   ij.TeamOwner,
			Plan:        appTypes.Plan{Name: ij.Plan},
			Name:        ij.Name,
			Description: ij.Description,
			Pool:        ij.Pool,
			Metadata:    ij.Metadata,
		},
		Schedule:  ij.Schedule,
		Container: ij.Container,
	}
	if newJob.TeamOwner == "" {
		oldJob.TeamOwner, err = autoTeamOwner(ctx, t, permission.PermAppCreate)
		if err != nil {
			return err
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     jobTarget(newJob.Name),
		Kind:       permission.PermJobUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermJobReadEvents, contextsForJob(&newJob)...),
	})
	defer func() {
		evt.Done(err)
	}()
	err = job.UpdateJob(ctx, &newJob, oldJob, u)
	if err != nil {
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
	w.WriteHeader(http.StatusAccepted)
	w.Write(jsonMsg)
	return nil
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
		Schedule:  ij.Schedule,
		Container: ij.Container,
	}
	if j.TeamOwner == "" {
		j.TeamOwner, err = autoTeamOwner(ctx, t, permission.PermAppCreate)
		if err != nil {
			return err
		}
	}
	canCreate := permission.Check(t, permission.PermJobCreate,
		permission.Context(permTypes.CtxTeam, j.TeamOwner),
	)
	if !canCreate {
		return permission.ErrUnauthorized
	}
	u, err := auth.ConvertNewUser(t.User())
	if err != nil {
		return err
	}
	err = job.CreateJob(ctx, &j, u, ij.Trigger)
	if err != nil {
		if e, ok := err.(*jobTypes.JobCreationError); ok {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: e.Error()}
		}
		if err == jobTypes.ErrJobAlreadyExists {
			return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
		}
		return err
	}
	evt, err := event.New(&event.Opts{
		Target:     jobTarget(j.Name),
		Kind:       permission.PermJobCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermJobReadEvents, contextsForJob(&j)...),
	})
	defer func() {
		evt.Done(err)
	}()
	if err != nil {
		return err
	}
	msg := map[string]interface{}{
		"status":  "success",
		"jobName": j.Name,
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

// title: delete job
// path: /jobs
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

	j, err := getJob(ctx, ij.Name)
	if err != nil {
		return err
	}
	canDelete := permission.Check(t, permission.PermJobDelete,
		contextsForJob(j)...,
	)
	if !canDelete {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     jobTarget(j.Name),
		Kind:       permission.PermJobDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermJobReadEvents, contextsForJob(j)...),
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
		permission.Context(permTypes.CtxJob, job.Name),
		permission.Context(permTypes.CtxPool, job.Pool),
	)
}
