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
	"strings"
	"sync/atomic"
	"time"

	"github.com/ajg/form"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	apiTypes "github.com/tsuru/tsuru/types/api"
	"github.com/tsuru/tsuru/types/app"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	logTypes "github.com/tsuru/tsuru/types/log"
	permTypes "github.com/tsuru/tsuru/types/permission"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	check "gopkg.in/check.v1"
)

func (s *S) TestDeleteCronjobAdminAuthorized(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := jobTypes.Job{
		Name:      "this-is-a-cronjob",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j, user)
	c.Assert(err, check.IsNil)
	myJob, err := servicemanager.Job.GetByName(context.TODO(), j.Name)
	c.Assert(err, check.IsNil)
	ij := inputJob{
		Name:      "this-is-a-cronjob",
		TeamOwner: myJob.TeamOwner,
		Pool:      "test1",
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/jobs/%s", ij.Name), &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestDeleteCronjob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := &jobTypes.Job{
		Name:      "my-cron",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), j, user)
	c.Assert(err, check.IsNil)
	myJob, err := servicemanager.Job.GetByName(context.TODO(), j.Name)
	c.Assert(err, check.IsNil)
	ij := inputJob{
		Name:      myJob.Name,
		TeamOwner: myJob.TeamOwner,
		Pool:      myJob.Pool,
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/jobs/%s", ij.Name), &buffer)
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
	c.Assert(eventtest.EventDesc{
		Target: jobTarget("my-cron"),
		Owner:  token.GetUserName(),
		Kind:   "job.delete",
	}, eventtest.HasEvent)
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
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/jobs/%s", job.Name), &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Job unknown not found.\n")
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
			OriginalImageSrc: "busybox:1.28",
			Command:          []string{"/bin/sh", "-c", "echo Hello!"},
		},
		Schedule: "* * * * *",
		Manual:   false,
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
	var gotJob jobTypes.Job
	err = s.conn.Jobs().Find(bson.M{"name": jobName, "teamowner": s.team.Name}).One(&gotJob)
	c.Assert(err, check.IsNil)
	expectedJob := jobTypes.Job{
		Name:      obtained["jobName"],
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Owner:     "majortom@groundcontrol.com",
		Plan: app.Plan{
			Name:    "default-plan",
			Memory:  1024,
			Default: true,
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
		Spec: jobTypes.JobSpec{
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "busybox:1.28",
				Command:          []string{"/bin/sh", "-c", "echo Hello!"},
			},
			Schedule:    "* * * * *",
			Manual:      false,
			ServiceEnvs: []bindTypes.ServiceEnvVar{},
			Envs:        []bindTypes.EnvVar{},
			ActiveDeadlineSeconds: func() *int64 {
				v := int64(0)
				return &v
			}(),
		},
	}
	c.Assert(gotJob, check.DeepEquals, expectedJob)
}

func (s *S) TestCreateManualJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := inputJob{
		Name:      "manual-job",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Plan:      "default-plan",
		Container: jobTypes.ContainerInfo{
			OriginalImageSrc: "busybox:1.28",
			Command:          []string{"/bin/sh", "-c", "echo Hello!"},
		},
		ActiveDeadlineSeconds: func() *int64 { i := int64(-1); return &i }(),
		Manual:                true,
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
	var gotJob jobTypes.Job
	err = s.conn.Jobs().Find(bson.M{"name": jobName, "teamowner": s.team.Name}).One(&gotJob)
	c.Assert(err, check.IsNil)
	expectedJob := jobTypes.Job{
		Name:      obtained["jobName"],
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Owner:     "majortom@groundcontrol.com",
		Plan: app.Plan{
			Name:    "default-plan",
			Memory:  1024,
			Default: true,
		},
		Pool: "test1",
		Metadata: app.Metadata{
			Labels:      []appTypes.MetadataItem{},
			Annotations: []appTypes.MetadataItem{},
		},
		Spec: jobTypes.JobSpec{
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "busybox:1.28",
				Command:          []string{"/bin/sh", "-c", "echo Hello!"},
			},
			Schedule:    "* * 31 2 *",
			Manual:      true,
			ServiceEnvs: []bindTypes.ServiceEnvVar{},
			Envs:        []bindTypes.EnvVar{},
			ActiveDeadlineSeconds: func() *int64 {
				v := int64(0)
				return &v
			}(),
		},
	}
	c.Assert(gotJob, check.DeepEquals, expectedJob)
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

func (s *S) TestCreateCronjobAndManualReturnConflict(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := inputJob{Name: "manualAndCronjob", TeamOwner: s.team.Name, Schedule: "* * * * *", Manual: true}
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
	c.Assert(recorder.Body.String(), check.Equals, "you can't set schedule and manual job at the same time\n")
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
}

func (s *S) TestUpdateCronjob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := jobTypes.Job{
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Name:      "cron",
		Spec: jobTypes.JobSpec{
			Schedule:              "* * * * *",
			ActiveDeadlineSeconds: func() *int64 { i := int64(36); return &i }(),
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	gotJob, err := servicemanager.Job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(gotJob.Spec.Container, check.DeepEquals, jobTypes.ContainerInfo{Command: []string{}})
	c.Assert(gotJob.Spec.Schedule, check.DeepEquals, "* * * * *")
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
			OriginalImageSrc: "busybox:1.28",
			Command:          []string{"/bin/sh", "-c", "echo Hello!"},
		},
		Schedule: "*/15 * * * *",
		ActiveDeadlineSeconds: func() *int64 {
			v := int64(0)
			return &v
		}(),
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s", ij.Name), &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusAccepted)
	gotJob, err = servicemanager.Job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	expectedJob := jobTypes.Job{
		Name:      j1.Name,
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Owner:     "super-root-toremove@groundcontrol.com",
		Plan: app.Plan{
			Name:    "default-plan",
			Memory:  1024,
			Default: true,
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
		Spec: jobTypes.JobSpec{
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "busybox:1.28",
				Command:          []string{"/bin/sh", "-c", "echo Hello!"},
			},
			Schedule:    "*/15 * * * *",
			ServiceEnvs: []bindTypes.ServiceEnvVar{},
			Envs:        []bindTypes.EnvVar{},
			ActiveDeadlineSeconds: func() *int64 {
				v := int64(0)
				return &v
			}(),
		},
	}
	c.Assert(*gotJob, check.DeepEquals, expectedJob)
}

func (s *S) TestKillJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := jobTypes.Job{
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Name:      "job1",
		Spec: jobTypes.JobSpec{
			Schedule:              "* * * * *",
			ActiveDeadlineSeconds: func() *int64 { i := int64(36); return &i }(),
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	gotJob, err := servicemanager.Job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(gotJob.Spec.Container, check.DeepEquals, jobTypes.ContainerInfo{Command: []string{}})
	c.Assert(gotJob.Spec.Schedule, check.DeepEquals, "* * * * *")
	var buffer bytes.Buffer
	request, err := http.NewRequest("DELETE", "/jobs/job1/units/unit2", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestKillJobUnitNotFound(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := jobTypes.Job{
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Name:      "job1",
		Spec: jobTypes.JobSpec{
			Schedule:              "* * * * *",
			ActiveDeadlineSeconds: func() *int64 { i := int64(36); return &i }(),
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	gotJob, err := servicemanager.Job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(gotJob.Spec.Container, check.DeepEquals, jobTypes.ContainerInfo{Command: []string{}})
	c.Assert(gotJob.Spec.Schedule, check.DeepEquals, "* * * * *")
	var buffer bytes.Buffer
	request, err := http.NewRequest("DELETE", "/jobs/job1/units/unit1", &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
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
			OriginalImageSrc: "ubuntu:latest",
			Command:          []string{"echo", "hello world"},
		},
		Schedule: "* * * */15 *",
	}
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s", ij.Name), &buffer)
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
	j1 := jobTypes.Job{
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Name:      "cron",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	_, err = servicemanager.Job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	ij := inputJob{
		Name:     "cron",
		Schedule: "invalid",
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s", ij.Name), &buffer)
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
	j1 := jobTypes.Job{
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Name:      "cron",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	_, err = servicemanager.Job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	ij := inputJob{
		Name:      "cron",
		TeamOwner: "invalid",
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s", ij.Name), &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.DeepEquals, "Job team owner \"invalid\" has no access to pool \"test1\"\n")
}

func (s *S) TestUpdateCronjobAndManualReturnConflict(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := jobTypes.Job{
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Name:      "cron",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	_, err = servicemanager.Job.GetByName(context.TODO(), j1.Name)
	c.Assert(err, check.IsNil)
	ij := inputJob{
		Name:     "cron",
		Manual:   true,
		Schedule: "*/5 * * * *",
	}
	var buffer bytes.Buffer
	err = json.NewEncoder(&buffer).Encode(ij)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s", ij.Name), &buffer)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.DeepEquals, "you can't set schedule and manual job at the same time\n")
}

func (s *S) TestTriggerCronjob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := jobTypes.Job{
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Name:      "manual-job",
		Spec: jobTypes.JobSpec{
			Schedule: "* */15 * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "ubuntu:latest",
				Command:          []string{"echo", "hello world"},
			},
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", fmt.Sprintf("/jobs/%s/trigger", j1.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestJobList(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := jobTypes.Job{
		Name:      "j1",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	j2 := jobTypes.Job{
		Name:      "j2",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "*/2 * * * *",
		},
	}
	j3 := jobTypes.Job{
		Name:      "j3",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "*/3 * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j2, user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j3, user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/jobs", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	jobs := []jobTypes.Job{}
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
	j1 := jobTypes.Job{
		Name:      "j1",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	j2 := jobTypes.Job{
		Name:      "j2",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "*/2 * * * *",
		},
	}
	j3 := jobTypes.Job{
		Name:      "j3",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "*/3 * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j2, user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j3, user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/jobs?name=j3", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	jobs := []jobTypes.Job{}
	err = json.Unmarshal(recorder.Body.Bytes(), &jobs)
	c.Assert(err, check.IsNil)
	c.Assert(len(jobs), check.Equals, 1)
	c.Assert(jobs[0].Name, check.Equals, "j3")
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
	j1 := jobTypes.Job{
		Name:      "j1",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	j2 := jobTypes.Job{
		Name:      "j2",
		TeamOwner: team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "*/2 * * * *",
		},
	}
	j3 := jobTypes.Job{
		Name:      "j3",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "*/3 * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j2, user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j3, user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/jobs?teamOwner=angra", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	jobs := []jobTypes.Job{}
	err = json.Unmarshal(recorder.Body.Bytes(), &jobs)
	c.Assert(err, check.IsNil)
	c.Assert(len(jobs), check.Equals, 1)
	c.Assert(jobs[0].Name, check.Equals, "j2")
}

func (s *S) TestJobListFilterByOwner(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	u, _ := token.User()
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j1 := jobTypes.Job{
		Name:      "j1",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	j2 := jobTypes.Job{
		Name:      "j2",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "*/2 * * * *",
		},
	}
	j3 := jobTypes.Job{
		Name:      "j3",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "*/3 * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j2, user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j3, u)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", fmt.Sprintf("/jobs?owner=%s", u.Email), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	jobs := []jobTypes.Job{}
	err = json.Unmarshal(recorder.Body.Bytes(), &jobs)
	c.Assert(err, check.IsNil)
	c.Assert(len(jobs), check.Equals, 1)
	c.Assert(jobs[0].Name, check.Equals, "j3")
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
	j1 := jobTypes.Job{
		Name:      "j1",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	j2 := jobTypes.Job{
		Name:      "j2",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "*/2 * * * *",
		},
	}
	j3 := jobTypes.Job{
		Name:      "j3",
		TeamOwner: s.team.Name,
		Pool:      "test1",
		Spec: jobTypes.JobSpec{
			Schedule: "*/3 * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j2, user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j3, user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/jobs?pool=pool1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	jobs := []jobTypes.Job{}
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

	prevClusterService := servicemanager.Cluster
	servicemanager.Cluster = &provTypes.MockClusterService{
		OnFindByPool: func(provisioner, pool string) (*provTypes.Cluster, error) {

			c.Assert(provisioner, check.Equals, "jobProv")
			c.Assert(pool, check.Equals, "pool1")

			return &provTypes.Cluster{
				Name: "cluster1",
			}, nil
		},
	}

	defer func() {
		servicemanager.Cluster = prevClusterService
	}()

	sInstance := service.ServiceInstance{
		Name:        "j1sql",
		ServiceName: "mysql",
		Tags:        []string{},
		Teams:       []string{s.team.Name},
		Jobs:        []string{"j1"},
	}
	err = s.conn.ServiceInstances().Insert(&sInstance)
	c.Assert(err, check.IsNil)

	defer provision.Unregister("jobProv")
	j1 := jobTypes.Job{
		Name:      "j1",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j1, user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", fmt.Sprintf("/jobs/%s", j1.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result jobInfoResult
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Cluster, check.Equals, "cluster1")
	c.Assert(s.team.Name, check.DeepEquals, result.Job.TeamOwner)
	c.Assert(j1.Pool, check.DeepEquals, result.Job.Pool)
	c.Assert("default-plan", check.DeepEquals, result.Job.Plan.Name)
	c.Assert([]string{s.team.Name}, check.DeepEquals, result.Job.Teams)
	c.Assert(s.user.Email, check.DeepEquals, result.Job.Owner)
	c.Assert([]bindTypes.ServiceInstanceBind{
		{Service: "mysql", Instance: "j1sql", Plan: ""},
	}, check.DeepEquals, result.ServiceInstanceBinds)
}

func (s *S) TestSuccessfulJobServiceInstanceBind(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"admin","DATABASE_PASSWORD":"secret"}`))
	}))
	defer ts.Close()

	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "secret", OwnerTeams: []string{s.team.Name}}
	err = service.Create(srvc)
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, instance.Name, job.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusOK)
	c.Check(recorder.Body.String(), check.Equals, "")

	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Jobs, check.DeepEquals, []string{job.Name})
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(job.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "job.update",
	}, eventtest.HasEvent)
}

func (s *S) TestJobServiceInstanceBindWithNonExistentServiceInstance(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, "fake-mysql", job.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusNotFound)
	c.Check(recorder.Body.String(), check.Equals, "service instance not found\n")
}

func (s *S) TestJobServiceInstanceBindServiceInstanceUpdateUnauthorized(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		TeamOwner:   s.team.Name,
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, instance.Name, job.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobUpdate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateBind,
		Context: permission.Context(permTypes.CtxServiceInstance, "invalid-team"),
	})
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusForbidden)
	c.Check(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
}

func (s *S) TestJobServiceInstanceBindWithNonExistentJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
	}
	err := s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, instance.Name, "fake-job-name")
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusNotFound)
	c.Check(recorder.Body.String(), check.Equals, "Job fake-job-name not found.\n")
}

func (s *S) TestJobServiceInstanceBindJobUpdateUnauthorized(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, instance.Name, job.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermJobUpdate,
		Context: permission.Context(permTypes.CtxTeam, "invalid-team"),
	})
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusForbidden)
	c.Check(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
}

func (s *S) TestJobServiceInstanceBindWithInvalidPoolService(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	s.mockService.Pool.OnServices = func(pool string) ([]string, error) {
		return []string{}, nil
	}

	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, instance.Name, job.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Check(recorder.Body.String(), check.Equals, "service \"mysql\" is not available for pool \"pool1\".\n")
}

func (s *S) TestJobServiceInstanceBindFailedToBindServiceInstanceToJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "secret", OwnerTeams: []string{s.team.Name}}
	err = service.Create(srvc)
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, instance.Name, job.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Check(recorder.Body.String(), check.Equals, "Failed to bind the instance \"mysql/my-mysql\" to the job \"test-job\": invalid response:  (code: 500) (\"my-mysql\" is down)\n")
}

func (s *S) TestSuccessfulJobServiceInstanceUnbind(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/binds/jobs/test-job" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()

	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "secret", OwnerTeams: []string{s.team.Name}}
	err = service.Create(srvc)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "my-mysql", ServiceName: "mysql"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "my-mysql", ServiceName: "mysql"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "fakehost"}, InstanceName: "our-mysql", ServiceName: "mysql"},
			},
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Jobs:        []string{job.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, instance.Name, job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)

	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Jobs, check.DeepEquals, []string{})

	createdJob, err := servicemanager.Job.GetByName(context.TODO(), job.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob.Spec.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "fakehost"}, InstanceName: "our-mysql", ServiceName: "mysql"},
	})

	ch := make(chan bool)
	go func() {
		t := time.Tick(1)
		for <-t; atomic.LoadInt32(&called) == 0; <-t {
		}
		ch <- true
	}()
	select {
	case <-ch:
		c.Succeed()
	case <-time.After(1e9):
		c.Error("Failed to call API after 1 second.")
	}

	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 3)
	c.Assert(parts[0], check.Matches, `{"Message":".*---- Unsetting 2 environment variables ----\\n","Timestamp":".*"}`)
	c.Assert(parts[1], check.Matches, `{"Message":".*\\n.*Instance \\"my-mysql\\" is not bound to the job \\"test-job\\" anymore.\\n","Timestamp":".*"}`)
	c.Assert(parts[2], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(job.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "job.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":job", "value": job.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSuccessfulForceJobServiceInstanceUnbind(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/binds/jobs/test-job" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("unbind error"))
		}
	}))
	defer ts.Close()

	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err = service.Create(srvc)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "my-mysql", ServiceName: "mysql"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "my-mysql", ServiceName: "mysql"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "fakehost"}, InstanceName: "our-mysql", ServiceName: "mysql"},
			},
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Jobs:        []string{"test-job"},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s?force=true", instance.ServiceName, instance.Name, job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)

	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Jobs, check.DeepEquals, []string{})

	createdJob, err := servicemanager.Job.GetByName(context.TODO(), job.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob.Spec.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "fakehost"}, InstanceName: "our-mysql", ServiceName: "mysql"},
	})

	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 4)
	c.Assert(parts[0], check.Matches, `{"Message":".*\[unbind-job-endpoint\] ignored error due to force: Failed to unbind \(\\"/resources/my-mysql/binds/jobs/test-job\\"\): invalid response: unbind error \(code: 500\)\\n","Timestamp":".*"}`)
	c.Assert(parts[1], check.Matches, `{"Message":".*---- Unsetting 2 environment variables ----\\n","Timestamp":".*"}`)
	c.Assert(parts[2], check.Matches, `{"Message":".*\\n.*Instance \\"my-mysql\\" is not bound to the job \\"test-job\\" anymore.\\n","Timestamp":".*"}`)
	c.Assert(parts[3], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(job.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "job.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":job", "value": job.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "force", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestJobServiceInstanceUnbindWithSameInstanceName(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/binds/jobs/test-job" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	srvcs := []service.Service{
		{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "secret", OwnerTeams: []string{s.team.Name}},
		{Name: "mysql2", Endpoint: map[string]string{"production": ts.URL}, Password: "secret", OwnerTeams: []string{s.team.Name}},
	}
	for _, srvc := range srvcs {
		err = service.Create(srvc)
		c.Assert(err, check.IsNil)
	}

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "my-mysql", ServiceName: "mysql"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "fakehost"}, InstanceName: "my-mysql", ServiceName: "mysql2"},
			},
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	instances := []service.ServiceInstance{
		{
			Name:        "my-mysql",
			ServiceName: "mysql",
			Teams:       []string{s.team.Name},
			Jobs:        []string{job.Name},
		},
		{
			Name:        "my-mysql",
			ServiceName: "mysql2",
			Teams:       []string{s.team.Name},
			Jobs:        []string{job.Name},
		},
	}
	for _, instance := range instances {
		err = s.conn.ServiceInstances().Insert(instance)
		c.Assert(err, check.IsNil)
	}

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instances[0].ServiceName, instances[0].Name, job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)

	var result service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{"name": instances[0].Name, "service_name": instances[0].ServiceName}).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Jobs, check.DeepEquals, []string{})

	err = s.conn.ServiceInstances().Find(bson.M{"name": instances[1].Name, "service_name": instances[1].ServiceName}).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Jobs, check.DeepEquals, []string{job.Name})
}

func (s *S) TestJobServiceInstanceUnbindWithNonExistentJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
	}
	err := s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, instance.Name, "fake-job-name")
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusNotFound)
	c.Check(recorder.Body.String(), check.Equals, "Job fake-job-name not found.\n")
}

func (s *S) TestJobServiceInstanceUnbindWithNonExistentServiceInstance(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, "fake-mysql", job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusNotFound)
	c.Check(recorder.Body.String(), check.Equals, "service instance not found\n")
}

func (s *S) TestJobServiceInstanceUnbindServiceInstanceUpdateUnauthorized(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		TeamOwner:   s.team.Name,
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, instance.Name, job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobUpdate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateBind,
		Context: permission.Context(permTypes.CtxServiceInstance, "invalid-team"),
	})
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusForbidden)
	c.Check(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
}

func (s *S) TestJobServiceInstanceUnbindJobUpdateUnauthorized(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, instance.Name, job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermJobUpdate,
		Context: permission.Context(permTypes.CtxTeam, "invalid-team"),
	})
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusForbidden)
	c.Check(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
}

func (s *S) TestSuccessfulForceJobServiceInstanceUnbindUnauthorized(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/binds/jobs/test-job" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("unbind error"))
		}
	}))
	defer ts.Close()

	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": "fake-endpoint"}, Password: "secret", OwnerTeams: []string{s.team.Name}}
	err = service.Create(srvc)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Jobs:        []string{job.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s?force=true", instance.ServiceName, instance.Name, job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermJobUpdate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})

	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
}

func (s *S) TestJobServiceInstanceUnbindFailedToUnbindServiceInstanceFromJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "secret", OwnerTeams: []string{s.team.Name}}
	err = service.Create(srvc)
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Jobs:        []string{job.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/services/%s/instances/%s/jobs/%s", instance.ServiceName, instance.Name, job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Check(recorder.Body.String(), check.Equals, "Failed to unbind (\"/resources/my-mysql/binds/jobs/test-job\"): invalid response:  (code: 500)\n")
}

func (s *S) TestGetEnvsAllJobEnvs(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{Name: "MY_ENV", Value: "my-value", Public: true},
				{Name: "YOUR_ENV", Value: "your-value", Public: true},
				{Name: "THEIR_ENV", Value: "their-value", Public: true},
			},
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/jobs/%s/env", job.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	expected := []bindTypes.EnvVar{
		{Name: "MY_ENV", Value: "my-value", Public: true},
		{Name: "YOUR_ENV", Value: "your-value", Public: true},
		{Name: "THEIR_ENV", Value: "their-value", Public: true},
		{Name: "TSURU_SERVICES", Value: "{}", Public: false},
	}
	result := []bindTypes.EnvVar{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, len(expected))

	for _, r := range result {
		for _, e := range expected {
			if e.Name == r.Name {
				c.Check(e.Public, check.Equals, r.Public)
				c.Check(e.Value, check.Equals, r.Value)
			}
		}
	}
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestGetOneJobEnv(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{Name: "MY_ENV", Value: "my-value", Public: true},
				{Name: "YOUR_ENV", Value: "your-value", Public: true},
				{Name: "THEIR_ENV", Value: "their-value", Public: true},
			},
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/jobs/%s/env?env=MY_ENV", job.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	expected := []map[string]interface{}{{
		"name":   "MY_ENV",
		"value":  "my-value",
		"public": true,
		"alias":  "",
	}}
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestGetMultipleJobEnvs(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{Name: "MY_ENV", Value: "my-value", Public: true},
				{Name: "YOUR_ENV", Value: "your-value", Public: true},
				{Name: "THEIR_ENV", Value: "their-value", Public: true},
			},
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/jobs/%s/env?env=MY_ENV&env=THEIR_ENV", job.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-type"), check.Equals, "application/json")

	expected := []map[string]interface{}{
		{"name": "MY_ENV", "value": "my-value", "public": true, "alias": ""},
		{"name": "THEIR_ENV", "value": "their-value", "public": true, "alias": ""},
	}
	var got []map[string]interface{}
	err = json.Unmarshal(recorder.Body.Bytes(), &got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestGetEnvJobDoesNotExist(c *check.C) {
	request, err := http.NewRequest("GET", "/jobs/unknown/env", nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Job unknown not found.\n")
}

func (s *S) TestGetJobEnvUserDoesNotHaveAccessToTheJob(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{Name: "MY_ENV", Value: "my-value", Public: true},
			},
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobRead,
		Context: permission.Context(permTypes.CtxJob, "-invalid-"),
	})
	url := fmt.Sprintf("/jobs/%s/env?envs=MY_ENV", job.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestJobEnvPublicEnvironmentVariableInTheJob(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")
	j := &jobTypes.Job{Name: "black-dog", TeamOwner: s.team.Name, Pool: "pool1", Spec: jobTypes.JobSpec{Schedule: "* * * * *"}}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), j, user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/jobs/%s/env", j.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	j, err = servicemanager.Job.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)
	expected := bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	c.Assert(j.Spec.Envs[0], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 1 new environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(j.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "job.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": j.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "localhost"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetJobEnvPrivateEnvironmentVariableInTheJob(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")

	job := jobTypes.Job{
		Name:      "test-job",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/jobs/%s/env", job.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_PASSWORD", Value: "secret", Alias: ""},
		},
		NoRestart: false,
		Private:   true,
	}

	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)

	buffer := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, buffer)
	c.Assert(err, check.IsNil)

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	createdJob, err := servicemanager.Job.GetByName(context.TODO(), job.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob.Spec.Envs[0], check.DeepEquals, bindTypes.EnvVar{
		Name: "DATABASE_PASSWORD", Value: "secret", Public: false,
	})
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 1 new environment variables ----\\n","Timestamp":".*"}\n`,
	)
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(job.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "job.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": job.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_PASSWORD"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "Private", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetJobEnvSetMultipleEnvironmentVariablesInTheJob(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")

	job := jobTypes.Job{
		Name:      "test-job",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/jobs/%s/env", job.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
			{Name: "DATABASE_USER", Value: "root", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)

	buffer := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, buffer)
	c.Assert(err, check.IsNil)

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	createdJob, err := servicemanager.Job.GetByName(context.TODO(), job.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob.Spec.Envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "DATABASE_HOST", Value: "localhost", Public: true},
		{Name: "DATABASE_USER", Value: "root", Public: true},
	})
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(job.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "job.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": job.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "localhost"},
			{"name": "Envs.1.Name", "value": "DATABASE_USER"},
			{"name": "Envs.1.Value", "value": "root"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetJobEnvNotToChangeValueOfServiceVariables(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")

	job := jobTypes.Job{
		Name:      "test-job",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{Name: "DATABASE_HOST", Value: "envhost", Public: true},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "servicehost"}, InstanceName: "myinstance", ServiceName: "srv1"},
			},
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/jobs/%s/env", job.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "newhost", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)

	buffer := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, buffer)
	c.Assert(err, check.IsNil)

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	createdJob, err := servicemanager.Job.GetByName(context.TODO(), job.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob.Spec.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "servicehost"}, InstanceName: "myinstance", ServiceName: "srv1"},
	})
	c.Assert(createdJob.Spec.Envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "DATABASE_HOST", Value: "newhost", Public: true},
	})
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(job.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "job.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": job.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "newhost"},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetBindEnvMissingFormBody(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")

	job := jobTypes.Job{
		Name:      "test-job",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/jobs/%s/env", job.Name)
	request, err := http.NewRequest("POST", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Matches, ".*missing form body\n")
}

func (s *S) TestSetJobEnvReturnsBadRequestIfVariablesAreMissing(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
	}

	url := fmt.Sprintf("/jobs/%s/env", job.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(""))
	c.Assert(err, check.IsNil)

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide the list of environment variables\n")
}

func (s *S) TestSetJobEnvReturnsNotFoundIfTheJobDoesNotExist(c *check.C) {
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "newhost", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)

	buffer := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/jobs/unknown/env", buffer)
	c.Assert(err, check.IsNil)

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Job unknown not found.\n")
}

func (s *S) TestSetJobEnvReturnsForbiddenIfTheUserDoesNotHaveAccessToTheJob(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")

	job := jobTypes.Job{
		Name:      "test-job",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "@yearly",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobUpdate,
		Context: permission.Context(permTypes.CtxJob, "another-job"),
	})
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)

	buffer := strings.NewReader(v.Encode())
	url := fmt.Sprintf("/jobs/%s/env", job.Name)
	request, err := http.NewRequest("POST", url, buffer)
	c.Assert(err, check.IsNil)

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestSetJobEnvReturnsBadRequestWhenGivenInvalidEnvName(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")

	job := jobTypes.Job{
		Name:      "test-job",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/jobs/%s/env", job.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "INVALID ENV", Value: "value"},
		},
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)

	buffer := strings.NewReader(v.Encode())
	request, err := http.NewRequest(http.MethodPost, url, buffer)
	c.Assert(err, check.IsNil)

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestUnsetJobEnv(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{Name: "DATABASE_HOST", Value: "localhost", Public: true},
				{Name: "DATABASE_USER", Value: "admin", Public: true},
				{Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
			},
		},
	}

	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/jobs/%s/env?env=DATABASE_HOST", job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	createdJob, err := servicemanager.Job.GetByName(context.TODO(), job.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob.Spec.Envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		{Name: "DATABASE_USER", Value: "admin", Public: true},
	})

	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Unsetting 1 environment variables ----\\n","Timestamp":".*"}\n`,
	)
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(job.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "job.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": job.Name},
			{"name": "env", "value": "DATABASE_HOST"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetJobEnvRemovesMultipleEnvironmentVariables(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{Name: "DATABASE_HOST", Value: "localhost", Public: true},
				{Name: "DATABASE_USER", Value: "admin", Public: true},
				{Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
			},
		},
	}

	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/jobs/%s/env?env=DATABASE_HOST&env=DATABASE_USER", job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	createdJob, err := servicemanager.Job.GetByName(context.TODO(), job.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob.Spec.Envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
	})

	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Unsetting 2 environment variables ----\\n","Timestamp":".*"}\n`,
	)
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(job.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "job.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": job.Name},
			{"name": "env", "value": []string{"DATABASE_HOST", "DATABASE_USER"}},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetJobEnvRemovesPrivateVariables(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
			},
		},
	}

	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/jobs/%s/env?env=DATABASE_PASSWORD", job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	createdJob, err := servicemanager.Job.GetByName(context.TODO(), job.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob.Spec.Envs, check.DeepEquals, []bindTypes.EnvVar{})

	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Unsetting 1 environment variables ----\\n","Timestamp":".*"}\n`,
	)
	c.Assert(eventtest.EventDesc{
		Target: jobTarget(job.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "job.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": job.Name},
			{"name": "env", "value": "DATABASE_PASSWORD"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetJobEnvReturnsBadRequestWhenVariablesMissing(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Envs: []bindTypes.EnvVar{
				{Name: "DATABASE_HOST", Value: "fakehost", Public: false},
			},
		},
	}
	url := fmt.Sprintf("/jobs/%s/env?env=", job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide the list of environment variables.\n")
}

func (s *S) TestUnsetJobEnvReturnsNotFoundWhenJobDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/jobs/unknown/env?env=ble", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Job unknown not found.\n")
}

func (s *S) TestUnsetJobEnvReturnsForbiddenWhenUserDoesNotHaveAccessToTheJob(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)

	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return &provisiontest.JobProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("jobProv")

	job := jobTypes.Job{
		Name:      "test-job",
		Pool:      "pool1",
		TeamOwner: s.team.Name,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{Name: "DATABASE_HOST", Value: "fakehost", Public: false},
			},
		},
	}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err = servicemanager.Job.CreateJob(context.TODO(), &job, user)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobUpdate,
		Context: permission.Context(permTypes.CtxJob, "another-job"),
	})
	url := fmt.Sprintf("/jobs/%s/env?env=DATABASE_HOST", job.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestJobLogShouldReturnNotFoundWhenJobDoesNotExist(c *check.C) {
	request, err := http.NewRequest("GET", "/jobs/unknown/log/?lines=10", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestJobLogReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheJob(c *check.C) {
	j := jobTypes.Job{Name: "lost", Pool: "test1"}
	err := s.conn.Jobs().Insert(j)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermJobRead,
		Context: permission.Context(permTypes.CtxTeam, "no-access"),
	})
	request, err := http.NewRequest("GET", fmt.Sprintf("/jobs/%s/log?lines=10", j.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestJobLogsList(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	j := jobTypes.Job{Name: "lost1", Pool: s.Pool, TeamOwner: s.team.Name, Spec: jobTypes.JobSpec{Schedule: "* * * * *"}}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j, user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", fmt.Sprintf("/jobs/%s/log?lines=10", j.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	var logs []appTypes.Applog
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs[0].Message, check.Equals, "Fake message from provisioner")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestJobLogsWatch(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "jobProv"
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		prov := provisiontest.ProvisionerInstance
		prov.LogsEnabled = true
		return &provisiontest.JobProvisioner{FakeProvisioner: prov}, nil
	})
	defer provision.Unregister("jobProv")
	j := jobTypes.Job{Name: "j1", Pool: s.Pool, TeamOwner: s.team.Name, Spec: jobTypes.JobSpec{Schedule: "* * * * *"}}
	user, _ := auth.ConvertOldUser(s.user, nil)
	err := servicemanager.Job.CreateJob(context.TODO(), &j, user)
	c.Assert(err, check.IsNil)
	logWatcher, err := s.provisioner.WatchLogs(context.TODO(), &j, appTypes.ListLogArgs{
		Name: j.Name,
		Type: logTypes.LogTypeJob,
	})
	c.Assert(err, check.IsNil)
	c.Assert(<-logWatcher.Chan(), check.DeepEquals, appTypes.Applog{
		Message: "Fake message from provisioner",
	})
	enc := &fakeEncoder{done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		logWatcher.(*app.MockLogWatcher).Enqueue(appTypes.Applog{Message: "xyz"})
		<-enc.done
		cancel()
	}()
	err = followLogs(ctx, j.Name, logWatcher, enc)
	c.Assert(err, check.IsNil)
	msgSlice, ok := enc.msg.([]appTypes.Applog)
	c.Assert(ok, check.Equals, true)
	c.Assert(msgSlice, check.DeepEquals, []appTypes.Applog{{Message: "xyz"}})
}
