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

	"github.com/tsuru/tsuru/job"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	check "gopkg.in/check.v1"
)

// func (s *S) TestJobListFilteringByPool(c *check.C) {
// 	opts := []pool.AddPoolOptions{
// 		{Name: "pool1", Default: false, Public: true},
// 		{Name: "pool2", Default: false, Public: true},
// 	}
// 	for _, opt := range opts {
// 		err := pool.AddPool(context.TODO(), opt)
// 		c.Assert(err, check.IsNil)
// 	}
// 	app1 := app.App{Name: "app1", Platform: "zend", Pool: opts[0].Name, TeamOwner: s.team.Name, Tags: []string{"mytag"}}
// 	err := app.CreateApp(context.TODO(), &app1, s.user)
// 	c.Assert(err, check.IsNil)
// 	app2 := app.App{Name: "app2", Platform: "zend", Pool: opts[1].Name, TeamOwner: s.team.Name, Tags: []string{""}}
// 	err = app.CreateApp(context.TODO(), &app2, s.user)
// 	c.Assert(err, check.IsNil)
// 	request, err := http.NewRequest("GET", fmt.Sprintf("/apps?pool=%s", opts[1].Name), nil)
// 	c.Assert(err, check.IsNil)
// 	request.Header.Set("Content-Type", "application/json")
// 	request.Header.Set("Authorization", "b "+s.token.GetValue())
// 	recorder := httptest.NewRecorder()
// 	s.testServer.ServeHTTP(recorder, request)
// 	c.Assert(recorder.Code, check.Equals, http.StatusOK)
// 	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
// 	apps := []app.App{}
// 	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
// 	c.Assert(err, check.IsNil)
// 	expected := []app.App{app2}
// 	c.Assert(apps, check.HasLen, len(expected))
// 	for i, app := range apps {
// 		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
// 		units, err := app.Units()
// 		c.Assert(err, check.IsNil)
// 		expectedUnits, err := expected[i].Units()
// 		c.Assert(err, check.IsNil)
// 		c.Assert(units, check.DeepEquals, expectedUnits)
// 		c.Assert(app.Tags, check.DeepEquals, expected[i].Tags)
// 	}
// }

// func (s *S) TestJobListByTeamOwner(c *check.C) {
// 	opts := []pool.AddPoolOptions{
// 		{Name: "pool1", Default: false, Public: true},
// 		{Name: "pool2", Default: false, Public: true},
// 	}
// 	for _, opt := range opts {
// 		err := pool.AddPool(context.TODO(), opt)
// 		c.Assert(err, check.IsNil)
// 	}
// 	app1 := app.App{Name: "app1", Platform: "zend", Pool: opts[0].Name, TeamOwner: s.team.Name, Tags: []string{"mytag"}}
// 	err := app.CreateApp(context.TODO(), &app1, s.user)
// 	c.Assert(err, check.IsNil)
// 	app2 := app.App{Name: "app2", Platform: "zend", Pool: opts[1].Name, TeamOwner: s.team.Name, Tags: []string{""}}
// 	err = app.CreateApp(context.TODO(), &app2, s.user)
// 	c.Assert(err, check.IsNil)
// 	request, err := http.NewRequest("GET", fmt.Sprintf("/apps?pool=%s", opts[1].Name), nil)
// 	c.Assert(err, check.IsNil)
// 	request.Header.Set("Content-Type", "application/json")
// 	request.Header.Set("Authorization", "b "+s.token.GetValue())
// 	recorder := httptest.NewRecorder()
// 	s.testServer.ServeHTTP(recorder, request)
// 	c.Assert(recorder.Code, check.Equals, http.StatusOK)
// 	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
// 	apps := []app.App{}
// 	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
// 	c.Assert(err, check.IsNil)
// 	expected := []app.App{app2}
// 	c.Assert(apps, check.HasLen, len(expected))
// 	for i, app := range apps {
// 		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
// 		units, err := app.Units()
// 		c.Assert(err, check.IsNil)
// 		expectedUnits, err := expected[i].Units()
// 		c.Assert(err, check.IsNil)
// 		c.Assert(units, check.DeepEquals, expectedUnits)
// 		c.Assert(app.Tags, check.DeepEquals, expected[i].Tags)
// 	}
// }

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

// func (s *S) TestDeleteJobMissingPool(c *check.C) {
// 	j := job.Job{
// 		TsuruJob: job.TsuruJob{
// 			Name:      "myjobtodelete",
// 			TeamOwner: s.team.Name,
// 		},
// 	}
// 	err := job.CreateJob(context.TODO(), &j, s.user)
// 	c.Assert(err, check.IsNil)
// 	myJob, err := job.GetByNameAndTeam(context.TODO(), j.Name, j.TeamOwner)
// 	c.Assert(err, check.IsNil)
// 	ij := inputJob{
// 		Name:      myJob.Name,
// 		TeamOwner: myJob.TeamOwner,
// 	}
// 	var buffer bytes.Buffer
// 	err = json.NewEncoder(&buffer).Encode(ij)
// 	c.Assert(err, check.IsNil)
// 	request, err := http.NewRequest("DELETE", "/jobs/", &buffer)
// 	c.Assert(err, check.IsNil)
// 	request.Header.Set("Authorization", "b "+s.token.GetValue())
// 	recorder := httptest.NewRecorder()
// 	s.testServer.ServeHTTP(recorder, request)
// 	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
// 	c.Assert(recorder.Body.String(), check.Equals, "Pool does not exist.\n")
// }

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

// func (s *S) TestJobInfo(c *check.C) {
// 	config.Set("host", "http://myhost.com")
// 	expectedApp := app.App{Name: "new-app", Platform: "zend", TeamOwner: s.team.Name}
// 	err := app.CreateApp(context.TODO(), &expectedApp, s.user)
// 	c.Assert(err, check.IsNil)
// 	var myApp map[string]interface{}
// 	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:app="+expectedApp.Name, nil)
// 	request.Header.Set("Content-Type", "application/json")
// 	request.Header.Set("Authorization", "b "+s.token.GetValue())
// 	recorder := httptest.NewRecorder()
// 	c.Assert(err, check.IsNil)
// 	role, err := permission.NewRole("reader", "app", "")
// 	c.Assert(err, check.IsNil)
// 	err = role.AddPermissions("app.read")
// 	c.Assert(err, check.IsNil)
// 	s.user.AddRole("reader", expectedApp.Name)
// 	s.testServer.ServeHTTP(recorder, request)
// 	c.Assert(recorder.Code, check.Equals, http.StatusOK)
// 	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
// 	err = json.Unmarshal(recorder.Body.Bytes(), &myApp)
// 	c.Assert(err, check.IsNil)
// 	c.Assert(myApp["name"], check.Equals, expectedApp.Name)
// }
