// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/job"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
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
	err := job.CreateJob(context.TODO(), &j, s.user, true)
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
	err := job.CreateJob(context.TODO(), &j, s.user, false)
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
	defer provision.Unregister("jobProv")
	j := &job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	err := job.CreateJob(context.TODO(), j, s.user, true)
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
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobDelete,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(myJob.Name),
		Owner:  token.GetUserName(),
		Kind:   "job.delete",
	}, eventtest.HasEvent)
}

func (s *S) TestDeleteJobForbidden(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := &job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	err := job.CreateJob(context.TODO(), j, s.user, true)
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
	token := userWithPermission(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestDeleteCronjob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := &job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "my-cron",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
		Schedule: "* * * * *",
	}
	err := job.CreateJob(context.TODO(), j, s.user, false)
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
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobDelete,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: jobTarget("my-cron"),
		Owner:  token.GetUserName(),
		Kind:   "job.delete",
	}, eventtest.HasEvent)
}

func (s *S) TestDeleteJobNotFound(c *check.C) {
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

func (s *S) TestDeleteCronjobNotFound(c *check.C) {
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

func (s *S) TestCreateSimpleJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := inputJob{TeamOwner: s.team.Name, Pool: "test1"}
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(j)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobCreate,
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
		Kind:   "job.create",
	}, eventtest.HasEvent)
}

func (s *S) TestCreateFullyFeaturedJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
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
		Container: jobTypes.ContainerInfo{
			Name:    "c1",
			Image:   "busybox:1.28",
			Command: []string{"/bin/sh", "-c", "echo Hello!"},
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
		Scheme:  permission.PermJobCreate,
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
		Container: jobTypes.ContainerInfo{
			Name:    "c1",
			Image:   "busybox:1.28",
			Command: []string{"/bin/sh", "-c", "echo Hello!"},
		},
	}
	c.Assert(gotJob, check.DeepEquals, expectedJob)
}

func (s *S) TestCreateFullyFeaturedCronjob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := inputJob{
		Name:        "full-cron",
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
		Container: jobTypes.ContainerInfo{
			Name:    "c1",
			Image:   "busybox:1.28",
			Command: []string{"/bin/sh", "-c", "echo Hello!"},
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
		Scheme:  permission.PermJobCreate,
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
		Container: jobTypes.ContainerInfo{
			Name:    "c1",
			Image:   "busybox:1.28",
			Command: []string{"/bin/sh", "-c", "echo Hello!"},
		},
		Schedule: "* * * * *",
	}
	c.Assert(gotJob, check.DeepEquals, expectedJob)
	c.Assert(gotJob.IsCron(), check.Equals, true)
}

func (s *S) TestCreateJobForbidden(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := inputJob{TeamOwner: s.team.Name, Pool: "test1"}
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(j)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestCreateJobAlreadyExists(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	oldJob := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "some-job",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	err := job.CreateJob(context.TODO(), &oldJob, s.user, true)
	c.Assert(err, check.IsNil)
	j := inputJob{Name: "some-job", TeamOwner: s.team.Name, Pool: "test1"}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(j)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "tsuru failed to create job \"some-job\": a job with the same name already exists\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestCreateJobNoPool(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := inputJob{Name: "some-job", TeamOwner: s.team.Name}
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(j)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "tsuru failed to create job \"some-job\": Pool does not exist.\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestCreateCronjobNoName(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := inputJob{TeamOwner: s.team.Name, Schedule: "* * * * *"}
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(j)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "tsuru failed to create job \"\": cronjob name can't be empty\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestUpdateJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	err := job.CreateJob(context.TODO(), &j1, s.user, true)
	c.Assert(err, check.IsNil)
	gotJob, err := job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(gotJob.Container, check.DeepEquals, jobTypes.ContainerInfo{Command: []string{}})
	ij := inputJob{
		Name: j1.Name,
		Container: jobTypes.ContainerInfo{
			Name:    "c1",
			Image:   "ubuntu:latest",
			Command: []string{"echo", "hello world"},
		},
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusAccepted)
	gotJob, err = job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(gotJob.Container, check.DeepEquals, ij.Container)
}

func (s *S) TestUpdateCronjob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
			Name:      "cron",
		},
		Schedule: "* * * * *",
	}
	err := job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	gotJob, err := job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(gotJob.Container, check.DeepEquals, jobTypes.ContainerInfo{Command: []string{}})
	c.Assert(gotJob.Schedule, check.DeepEquals, "* * * * *")
	ij := inputJob{
		Name:        j1.Name,
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
		Container: jobTypes.ContainerInfo{
			Name:    "c1",
			Image:   "busybox:1.28",
			Command: []string{"/bin/sh", "-c", "echo Hello!"},
		},
		Schedule: "*/15 * * * *",
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusAccepted)
	gotJob, err = job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	expectedJob := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      j1.Name,
			Teams:     []string{s.team.Name},
			TeamOwner: s.team.Name,
			Owner:     "super-root-toremove@groundcontrol.com",
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
		Container: jobTypes.ContainerInfo{
			Name:    "c1",
			Image:   "busybox:1.28",
			Command: []string{"/bin/sh", "-c", "echo Hello!"},
		},
		Schedule: "*/15 * * * *",
	}
	// we have to check the values 1 by 1 because the value of job.ctx is != nil
	c.Assert(gotJob.Name, check.DeepEquals, expectedJob.Name)
	c.Assert(gotJob.Teams, check.DeepEquals, expectedJob.Teams)
	c.Assert(gotJob.TeamOwner, check.DeepEquals, expectedJob.TeamOwner)
	c.Assert(gotJob.Owner, check.DeepEquals, expectedJob.Owner)
	c.Assert(gotJob.Plan, check.DeepEquals, expectedJob.Plan)
	c.Assert(gotJob.Metadata, check.DeepEquals, expectedJob.Metadata)
	c.Assert(gotJob.Pool, check.DeepEquals, expectedJob.Pool)
	c.Assert(gotJob.Description, check.DeepEquals, expectedJob.Description)
	c.Assert(gotJob.Container, check.DeepEquals, expectedJob.Container)
	c.Assert(gotJob.Schedule, check.DeepEquals, expectedJob.Schedule)
}

func (s *S) TestUpdateCronjobNotFound(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	ij := inputJob{
		Name: "i-dont-exist",
		Container: jobTypes.ContainerInfo{
			Name:    "c1",
			Image:   "ubuntu:latest",
			Command: []string{"echo", "hello world"},
		},
		Schedule: "* * * */15 *",
	}
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.DeepEquals, "Job i-dont-exist not found.\n")
}

func (s *S) TestUpdateCronjobInvalidSchedule(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
			Name:      "cron",
		},
		Schedule: "* * * * *",
	}
	err := job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	_, err = job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	ij := inputJob{
		Name:     "cron",
		Schedule: "invalid",
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.DeepEquals, "invalid schedule\n")
}

func (s *S) TestUpdateCronjobInvalidTeam(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
			Name:      "cron",
		},
		Schedule: "* * * * *",
	}
	err := job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	_, err = job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	ij := inputJob{
		Name:      "cron",
		TeamOwner: "invalid",
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/jobs", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.DeepEquals, "Job team owner \"invalid\" has no access to pool \"test1\"\n")
}

func (s *S) TestTriggerManualJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
			Name:      "manual-job",
		},
		Container: jobTypes.ContainerInfo{
			Name:    "c1",
			Image:   "ubuntu:latest",
			Command: []string{"echo", "hello world"},
		},
	}
	err := job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s/trigger", j1.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestTriggerCronjob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
			Name:      "manual-job",
		},
		Schedule: "* */15 * * *",
		Container: jobTypes.ContainerInfo{
			Name:    "c1",
			Image:   "ubuntu:latest",
			Command: []string{"echo", "hello world"},
		},
	}
	err := job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s/trigger", j1.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestTriggerJobNotFound(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	request, err := http.NewRequest("PUT", "/jobs/some-name/trigger", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestJobList(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	j2 := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "manual",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	j3 := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "cron",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
		Schedule: "* * * * *",
	}
	err := job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	err = job.CreateJob(context.TODO(), &j2, s.user, false)
	c.Assert(err, check.IsNil)
	err = job.CreateJob(context.TODO(), &j3, s.user, false)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/jobs", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	jobs := []job.Job{}
	err = json.Unmarshal(recorder.Body.Bytes(), &jobs)
	c.Assert(err, check.IsNil)
	c.Assert(len(jobs), check.Equals, 3)
}

func (s *S) TestJobListFilterByName(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	j2 := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "manual",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	j3 := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "cron",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
		Schedule: "* * * * *",
	}
	err := job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	err = job.CreateJob(context.TODO(), &j2, s.user, false)
	c.Assert(err, check.IsNil)
	err = job.CreateJob(context.TODO(), &j3, s.user, false)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/jobs?name=manual", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	jobs := []job.Job{}
	err = json.Unmarshal(recorder.Body.Bytes(), &jobs)
	c.Assert(err, check.IsNil)
	c.Assert(len(jobs), check.Equals, 1)
	c.Assert(jobs[0].Name, check.Equals, "manual")
}

func (s *S) TestJobListFilterByTeamowner(c *check.C) {
	team := authTypes.Team{Name: "angra"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		return &authTypes.Team{Name: name}, nil
	}
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	j2 := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "manual",
			TeamOwner: team.Name,
			Pool:      "test1",
		},
	}
	j3 := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "cron",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
		Schedule: "* * * * *",
	}
	err := job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	err = job.CreateJob(context.TODO(), &j2, s.user, false)
	c.Assert(err, check.IsNil)
	err = job.CreateJob(context.TODO(), &j3, s.user, false)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/jobs?teamOwner=angra", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	jobs := []job.Job{}
	err = json.Unmarshal(recorder.Body.Bytes(), &jobs)
	c.Assert(err, check.IsNil)
	c.Assert(len(jobs), check.Equals, 1)
	c.Assert(jobs[0].Name, check.Equals, "manual")
}

func (s *S) TestJobListFilterByOwner(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	u, _ := auth.ConvertNewUser(token.User())
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	j2 := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "manual",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	j3 := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "cron",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
		Schedule: "* * * * *",
	}
	err := job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	err = job.CreateJob(context.TODO(), &j2, s.user, false)
	c.Assert(err, check.IsNil)
	err = job.CreateJob(context.TODO(), &j3, u, false)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", fmt.Sprintf("/jobs?owner=%s", u.Email), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	jobs := []job.Job{}
	err = json.Unmarshal(recorder.Body.Bytes(), &jobs)
	c.Assert(err, check.IsNil)
	c.Assert(len(jobs), check.Equals, 1)
	c.Assert(jobs[0].Name, check.Equals, "cron")
}

func (s *S) TestJobListFilterPool(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "pool1",
		},
	}
	j2 := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "manual",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
	}
	j3 := job.Job{
		TsuruJob: job.TsuruJob{
			Name:      "cron",
			TeamOwner: s.team.Name,
			Pool:      "test1",
		},
		Schedule: "* * * * *",
	}
	err = job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	err = job.CreateJob(context.TODO(), &j2, s.user, false)
	c.Assert(err, check.IsNil)
	err = job.CreateJob(context.TODO(), &j3, s.user, false)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/jobs?pool=pool1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	jobs := []job.Job{}
	err = json.Unmarshal(recorder.Body.Bytes(), &jobs)
	c.Assert(err, check.IsNil)
	c.Assert(len(jobs), check.Equals, 1)
	c.Assert(jobs[0].Name, check.Equals, j1.Name)
}

func (s *S) TestJobInfo(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := job.Job{
		TsuruJob: job.TsuruJob{
			TeamOwner: s.team.Name,
			Pool:      "pool1",
		},
	}
	err = job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", fmt.Sprintf("/jobs/%s", j1.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result struct {
		Job   job.Job          `json:"job,omitempty"`
		Units []provision.Unit `json:"units,omitempty"`
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(s.team.Name, check.DeepEquals, result.Job.TeamOwner)
	c.Assert(j1.Pool, check.DeepEquals, result.Job.Pool)
	c.Assert("default-plan", check.DeepEquals, result.Job.Plan.Name)
	c.Assert([]string{s.team.Name}, check.DeepEquals, result.Job.Teams)
	c.Assert(s.user.Email, check.DeepEquals, result.Job.Owner)
}
