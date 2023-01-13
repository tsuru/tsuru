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

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/job"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	check "gopkg.in/check.v1"
)

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
	err := job.CreateJob(context.TODO(), &j, s.user, false)
	c.Assert(err, check.IsNil)
	myJob, err := job.GetByName(context.TODO(), j.Name)
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
	myJob, err := job.GetByName(context.TODO(), j.Name)
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
	myJob, err := job.GetByName(context.TODO(), j.Name)
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
	myJob, err := job.GetByName(context.TODO(), j.Name)
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

func (s *S) TestDeleteShouldReturnForbiddenIfTheGivenUserDoesNotHaveAccessToTheJob(c *check.C) {
	j := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "job1",
			TeamOwner: "admin",
		},
	}
	ij := inputJob{
		Name:      j.Name,
		TeamOwner: j.TeamOwner,
	}
	err := s.conn.Jobs().Insert(j)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppDelete,
		Context: permission.Context(permTypes.CtxApp, "-other-job-"),
	})
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestCreateSimpleJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	j := inputJob{TeamOwner: s.team.Name, Pool: "test1"}
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(j)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, token.GetUserName())
		return nil
	}
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained["status"], check.DeepEquals, "success")
	jobName, ok := obtained["jobName"]
	c.Assert(ok, check.Equals, true)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var gotJob job.Job
	err = s.conn.Jobs().Find(bson.M{"tsurujob.name": jobName, "tsurujob.teamowner": s.team.Name}).One(&gotJob)
	c.Assert(err, check.IsNil)
	c.Assert(gotJob.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(jobName),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
	}, eventtest.HasEvent)
}

func (s *S) TestCreateFullJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	j := inputJob{
		TeamOwner:   s.team.Name,
		Pool:        "test1",
		Plan:        "default-plan",
		Description: "some description",
		Metadata: app.Metadata{
			Labels: []app.MetadataItem{
				{
					Name:  "label1",
					Value: "value1",
				},
			},
			Annotations: []app.MetadataItem{
				{
					Name:  "annotation1",
					Value: "value2",
				},
			},
		},
		Containers: []jobTypes.ContainerInfo{
			{
				Name:    "c1",
				Image:   "busybox:1.28",
				Command: []string{"/bin/sh", "-c", "echo Hello!"},
			},
		},
	}
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(j)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, token.GetUserName())
		return nil
	}
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained["status"], check.DeepEquals, "success")
	jobName, ok := obtained["jobName"]
	c.Assert(ok, check.Equals, true)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var gotJob job.Job
	err = s.conn.Jobs().Find(bson.M{"tsurujob.name": jobName, "tsurujob.teamowner": s.team.Name}).One(&gotJob)
	c.Assert(err, check.IsNil)
	expectedJob := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      obtained["jobName"],
			Teams:     []string{s.team.Name},
			TeamOwner: s.team.Name,
			Owner:     "majortom@groundcontrol.com",
			Plan: app.Plan{
				Name:     "default-plan",
				Memory:   1024,
				Swap:     1024,
				CpuShare: 100,
				Default:  true,
			},
			Metadata: app.Metadata{
				Labels: []app.MetadataItem{
					{
						Name:  "label1",
						Value: "value1",
					},
				},
				Annotations: []app.MetadataItem{
					{
						Name:  "annotation1",
						Value: "value2",
					},
				},
			},
			Pool:        "test1",
			Description: "some description",
		},
		Containers: []jobTypes.ContainerInfo{
			{
				Name:    "c1",
				Image:   "busybox:1.28",
				Command: []string{"/bin/sh", "-c", "echo Hello!"},
			},
		},
	}
	c.Assert(gotJob, check.DeepEquals, expectedJob)
}

func (s *S) TestCreateFullCronjob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	j := inputJob{
		TeamOwner:   s.team.Name,
		Pool:        "test1",
		Plan:        "default-plan",
		Description: "some description",
		Metadata: app.Metadata{
			Labels: []app.MetadataItem{
				{
					Name:  "label1",
					Value: "value1",
				},
			},
			Annotations: []app.MetadataItem{
				{
					Name:  "annotation1",
					Value: "value2",
				},
			},
		},
		Containers: []jobTypes.ContainerInfo{
			{
				Name:    "c1",
				Image:   "busybox:1.28",
				Command: []string{"/bin/sh", "-c", "echo Hello!"},
			},
		},
		Schedule: "* * * * *",
	}
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(j)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, token.GetUserName())
		return nil
	}
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained["status"], check.DeepEquals, "success")
	jobName, ok := obtained["jobName"]
	c.Assert(ok, check.Equals, true)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var gotJob job.Job
	err = s.conn.Jobs().Find(bson.M{"tsurujob.name": jobName, "tsurujob.teamowner": s.team.Name}).One(&gotJob)
	c.Assert(err, check.IsNil)
	expectedJob := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      obtained["jobName"],
			Teams:     []string{s.team.Name},
			TeamOwner: s.team.Name,
			Owner:     "majortom@groundcontrol.com",
			Plan: app.Plan{
				Name:     "default-plan",
				Memory:   1024,
				Swap:     1024,
				CpuShare: 100,
				Default:  true,
			},
			Metadata: app.Metadata{
				Labels: []app.MetadataItem{
					{
						Name:  "label1",
						Value: "value1",
					},
				},
				Annotations: []app.MetadataItem{
					{
						Name:  "annotation1",
						Value: "value2",
					},
				},
			},
			Pool:        "test1",
			Description: "some description",
		},
		Containers: []jobTypes.ContainerInfo{
			{
				Name:    "c1",
				Image:   "busybox:1.28",
				Command: []string{"/bin/sh", "-c", "echo Hello!"},
			},
		},
		Schedule: "* * * * *",
	}
	c.Assert(gotJob, check.DeepEquals, expectedJob)
	c.Assert(gotJob.IsCron(), check.Equals, true)
}
