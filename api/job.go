// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	stdContext "context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/job"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/service"
	appTypes "github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

type inputJob struct {
	TeamOwner   string            `json:"teamOwner"`
	Plan        string            `json:"plan"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Pool        string            `json:"pool"`
	Metadata    appTypes.Metadata `json:"metadata"`

	Schedule  string                 `json:"schedule"`
	Container jobTypes.ContainerInfo `json:"container"`
	Trigger   bool                   `json:"trigger"` // Trigger means the client wants to forcefully run a job or a cronjob
}

func getJob(ctx stdContext.Context, name string) (*jobTypes.Job, error) {
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
	contexts := permission.ContextsForPermission(t, permission.PermJobRead)
	contexts = append(contexts, permission.ContextsForPermission(t, permission.PermJobRead)...)
	if len(contexts) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	jobs, err := job.List(ctx, jobFilterByContext(contexts, filter))
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
// produce: application/json
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
// produce: application/json
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
	w.Header().Set("Content-Type", "application/json")
	units, err := job.Units(ctx, j)
	if err != nil {
		return err
	}
	result := struct {
		Job   *jobTypes.Job    `json:"job,omitempty"`
		Units []provision.Unit `json:"units,omitempty"`
	}{
		Job:   j,
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
	name := r.URL.Query().Get(":name")
	var ij inputJob
	err = ParseInput(r, &ij)
	if err != nil {
		return err
	}
	oldJob, err := getJob(ctx, name)
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
	newJob := jobTypes.Job{
		TeamOwner:   ij.TeamOwner,
		Plan:        appTypes.Plan{Name: ij.Plan},
		Name:        ij.Name,
		Description: ij.Description,
		Pool:        ij.Pool,
		Metadata:    ij.Metadata,
		Spec: jobTypes.JobSpec{
			Schedule:  ij.Schedule,
			Container: ij.Container,
		},
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
	if err != nil {
		return err
	}
	defer func() {
		evt.Done(err)
	}()
	err = job.UpdateJob(ctx, &newJob, oldJob, u)
	if err != nil {
		return err
	}
	updatedJob, err := getJob(ctx, name)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	err = json.NewEncoder(w).Encode(updatedJob)
	if err != nil {
		return err
	}
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
	j := &jobTypes.Job{
		TeamOwner:   ij.TeamOwner,
		Plan:        appTypes.Plan{Name: ij.Plan},
		Name:        ij.Name,
		Description: ij.Description,
		Pool:        ij.Pool,
		Metadata:    ij.Metadata,
		Spec: jobTypes.JobSpec{
			Schedule:  ij.Schedule,
			Container: ij.Container,
		},
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
	err = job.CreateJob(ctx, j, u, ij.Trigger)
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
		Allowed:    event.Allowed(permission.PermJobReadEvents, contextsForJob(j)...),
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
// produce: application/json
// responses:
//
//	200: Job removed
//	401: Unauthorized
//	404: Not found
func deleteJob(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	name := r.URL.Query().Get(":name")
	var ij inputJob
	err = ParseInput(r, &ij)
	if err != nil {
		return err
	}
	j, err := getJob(ctx, name)
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
	if err = job.RemoveJobFromDb(j.Name); err != nil {
		if err == jobTypes.ErrJobNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	err = job.DeleteFromProvisioner(ctx, j)
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

// title: bind service instance to a job
// path: /services/{service}/instances/{instance}/jobs/{job}
// method: PUT
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//
//	200: Ok
//	400: Invalid data
//	401: Unauthorized
//	404: App not found
func bindJobServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	instanceName := r.URL.Query().Get(":instance")
	jobName := r.URL.Query().Get(":job")
	serviceName := r.URL.Query().Get(":service")

	instance, err := getServiceInstanceOrError(ctx, serviceName, instanceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateBind,
		append(permission.Contexts(permTypes.CtxTeam, instance.Teams),
			permission.Context(permTypes.CtxTeam, instance.TeamOwner),
			permission.Context(permTypes.CtxServiceInstance, instance.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	j, err := getJob(ctx, jobName)
	if err != nil {
		return err
	}
	canUpdate := permission.Check(t, permission.PermJobUpdate,
		contextsForJob(j)...,
	)
	if !canUpdate {
		return permission.ErrUnauthorized
	}

	err = pool.ValidatePoolService(ctx, j.Pool, []string{serviceName})
	if err != nil {
		if err == pool.ErrPoolHasNoService {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}
		return err
	}

	evt, err := event.New(&event.Opts{
		Target: jobTarget(j.Name),
		ExtraTargets: []event.ExtraTarget{
			{Target: serviceInstanceTarget(serviceName, instanceName)},
		},
		Kind:       permission.PermJobUpdate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForJob(j)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()

	err = instance.BindJob(j, evt, evt, requestIDHeader(r))
	if err != nil {
		status, errStatus := instance.Status(requestIDHeader(r))
		if errStatus != nil {
			return fmt.Errorf("%v (failed to retrieve instance status: %v)", err, errStatus)
		}
		return fmt.Errorf("%v (%q is %v)", err, instanceName, status)
	}
	return nil
}

// title: unbind service instance for a job
// path: /services/{service}/instances/{instance}/job/{job}
// method: DELETE
// produce: application/x-json-stream
// responses:
//
//	200: Ok
//	400: Invalid data
//	401: Unauthorized
//	404: Job not found
func unbindJobServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	instanceName := r.URL.Query().Get(":instance")
	jobName := r.URL.Query().Get(":job")
	serviceName := r.URL.Query().Get(":service")
	force, _ := strconv.ParseBool(InputValue(r, "force"))

	j, err := getJob(ctx, jobName)
	if err != nil {
		return err
	}

	instance, err := getServiceInstanceOrError(ctx, serviceName, instanceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateUnbind,
		append(permission.Contexts(permTypes.CtxTeam, instance.Teams),
			permission.Context(permTypes.CtxTeam, instance.TeamOwner),
			permission.Context(permTypes.CtxServiceInstance, instance.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	allowed = permission.Check(t, permission.PermJobUpdate,
		contextsForJob(j)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	if force {
		s, errGet := service.Get(ctx, instance.ServiceName)
		if errGet != nil {
			return errGet
		}
		allowed = permission.Check(t, permission.PermServiceUpdate,
			contextsForServiceProvision(&s)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	evt, err := event.New(&event.Opts{
		Target: jobTarget(jobName),
		ExtraTargets: []event.ExtraTarget{
			{Target: serviceInstanceTarget(serviceName, instanceName)},
		},
		Kind:       permission.PermAppUpdateUnbind,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForJob(j)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt.SetLogWriter(writer)
	err = instance.UnbindJob(service.UnbindJobArgs{
		Job:         j,
		ForceRemove: force,
		Event:       evt,
		RequestID:   requestIDHeader(r),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(evt, "\nInstance %q is not bound to the job %q anymore.\n", instanceName, jobName)
	return nil
}

func jobTarget(jobName string) event.Target {
	return event.Target{Type: event.TargetTypeJob, Value: jobName}
}

func contextsForJob(job *jobTypes.Job) []permTypes.PermissionContext {
	return append(permission.Contexts(permTypes.CtxTeam, job.Teams),
		permission.Context(permTypes.CtxJob, job.Name),
		permission.Context(permTypes.CtxPool, job.Pool),
	)
}
