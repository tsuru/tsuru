// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/job"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	check "gopkg.in/check.v1"
)

func (s *S) TestDeleteShouldReturnNotFoundIfTheJobDoesNotExist(c *check.C) {
	job := inputJob{
		Name:      "unknown",
		TeamOwner: "unknown",
	}
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(job)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Job unknown not found.\n")
}

func (s *S) TestDeleteShouldReturnNotFoundIfTheCronjobDoesNotExist(c *check.C) {
	job := inputJob{
		Name:      "unknown",
		TeamOwner: "unknown",
		Schedule:  "* * * * *",
	}
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(job)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Job unknown not found.\n")
}

func (s *S) TestDeleteJobAdminAuthorized(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	err := job.CreateJob(context.TODO(), &j, s.user)
	c.Assert(err, check.IsNil)
	myJob, err := job.GetByNameAndTeam(context.TODO(), j.Name, j.TeamOwner)
	c.Assert(err, check.IsNil)
	ij := inputJob{
		Name:      myJob.Name,
		TeamOwner: myJob.TeamOwner,
		Pool:      "test1",
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestDeleteCronjobAdminAuthorized(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "this-is-a-cronjob",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
		Schedule: "* * * * *",
	}
	err := job.CreateJob(context.TODO(), &j, s.user)
	c.Assert(err, check.IsNil)
	myJob, err := job.GetByNameAndTeam(context.TODO(), j.Name, j.TeamOwner)
	c.Assert(err, check.IsNil)
	ij := inputJob{
		Name:      "this-is-a-cronjob",
		TeamOwner: myJob.TeamOwner,
		Pool:      "test1",
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestDeleteJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	j := &job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	err := job.CreateJob(context.TODO(), j, s.user)
	c.Assert(err, check.IsNil)
	myJob, err := job.GetByNameAndTeam(context.TODO(), j.Name, j.TeamOwner)
	c.Assert(err, check.IsNil)
	ij := inputJob{
		Name:      myJob.Name,
		TeamOwner: myJob.TeamOwner,
		Pool:      myJob.Pool,
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	role, err := permission.NewRole("deleter", "app", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app.delete")
	c.Assert(err, check.IsNil)
	err = s.user.AddRole("deleter", myJob.Name)
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(myJob.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.delete",
	}, eventtest.HasEvent)
}

func (s *S) TestDeleteCronjob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	j := &job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "my-cron",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
		Schedule: "* * * * *",
	}
	err := job.CreateJob(context.TODO(), j, s.user)
	c.Assert(err, check.IsNil)
	myJob, err := job.GetByNameAndTeam(context.TODO(), j.Name, j.TeamOwner)
	c.Assert(err, check.IsNil)
	ij := inputJob{
		Name:      myJob.Name,
		TeamOwner: myJob.TeamOwner,
		Pool:      myJob.Pool,
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	role, err := permission.NewRole("deleter", "app", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app.delete")
	c.Assert(err, check.IsNil)
	err = s.user.AddRole("deleter", myJob.Name)
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: jobTarget("my-cron"),
		Owner:  s.token.GetUserName(),
		Kind:   "app.delete",
	}, eventtest.HasEvent)
}
