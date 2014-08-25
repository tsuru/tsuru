// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

type Deploy struct {
	ID        bson.ObjectId `bson:"_id,omitempty"`
	App       string
	Timestamp time.Time
	Duration  time.Duration
	Commit    string
	Error     string
}

type DeploySuite struct {
	conn        *db.Storage
	token       auth.Token
	team        *auth.Team
	provisioner *testing.FakeProvisioner
}

var _ = gocheck.Suite(&DeploySuite{})

func (s *DeploySuite) createUserAndTeam(c *gocheck.C) {
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	_, err := nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, gocheck.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
}

func (s *DeploySuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_deploy_api_tests")
	config.Set("aut:hash-cost", 4)
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	s.createUserAndTeam(c)
	s.provisioner = testing.NewFakeProvisioner()
	app.Provisioner = s.provisioner
}

func (s *DeploySuite) TearDownSuite(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *DeploySuite) TestDeployHandler(c *gocheck.C) {
	a := app.App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("version=a345f3e&user=fulano"))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "text")
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(b), gocheck.Equals, "Git deploy called\nOK\n")
	c.Assert(s.provisioner.Version(&a), gocheck.Equals, "a345f3e")
}

func (s *DeploySuite) TestDeployArchiveURL(c *gocheck.C) {
	a := app.App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz&user=fulano"))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "text")
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(b), gocheck.Equals, "Archive deploy called\nOK\n")
}

func (s *DeploySuite) TestDeployWithCommit(c *gocheck.C) {
	a := app.App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("version=a345f3e&user=fulano&commit=123"))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "text")
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(b), gocheck.Equals, "Git deploy called\nOK\n")
	deploys, err := s.conn.Deploys().Find(bson.M{"commit": "123"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(deploys, gocheck.Equals, 1)
	c.Assert(s.provisioner.Version(&a), gocheck.Equals, "a345f3e")
}

func (s *DeploySuite) TestDeployShouldIncrementDeployNumberOnApp(c *gocheck.C) {
	a := app.App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("version=a345f3e"))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, gocheck.Equals, uint(1))
	var result map[string]interface{}
	s.conn.Deploys().Find(bson.M{"app": a.Name}).One(&result)
	c.Assert(result["app"], gocheck.Equals, a.Name)
	now := time.Now()
	diff := now.Sub(result["timestamp"].(time.Time))
	c.Assert(diff < 60*time.Second, gocheck.Equals, true)
}

func (s *DeploySuite) TestDeployShouldReturnNotFoundWhenAppDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("POST", "/apps/abc/repository/clone?:appname=abc", strings.NewReader("version=abcdef"))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App abc not found.$")
}

func (s *DeploySuite) TestDeployWithoutVersionAndArchiveURL(c *gocheck.C) {
	request, err := http.NewRequest("POST", "/apps/abc/repository/clone?:appname=abc", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "you must specify either the version or the archive-url")
}

func (s *DeploySuite) TestDeployWithVersionAndArchiveURL(c *gocheck.C) {
	body := strings.NewReader("version=abcdef&archive-url=http://google.com")
	request, err := http.NewRequest("POST", "/apps/abc/repository/clone?:appname=abc", body)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "you must specify either the version or the archive-url, but not both")
}

func (s *DeploySuite) TestDeployList(c *gocheck.C) {
	var result []Deploy
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	request, err := http.NewRequest("GET", "/deploys", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	err = s.conn.Deploys().Insert(Deploy{App: "g1", Timestamp: timestamp.Add(time.Minute), Duration: duration})
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Deploys().Insert(Deploy{App: "ge", Timestamp: timestamp.Add(time.Second), Duration: duration})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Deploys().RemoveAll(nil)
	err = deploysList(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result[0].App, gocheck.Equals, "g1")
	c.Assert(result[0].Timestamp.In(time.UTC), gocheck.DeepEquals, timestamp.Add(time.Minute).In(time.UTC))
	c.Assert(result[0].Duration, gocheck.DeepEquals, duration)
	c.Assert(result[1].App, gocheck.Equals, "ge")
	c.Assert(result[1].Timestamp.In(time.UTC), gocheck.DeepEquals, timestamp.Add(time.Second).In(time.UTC))
	c.Assert(result[1].Duration, gocheck.DeepEquals, duration)
}

func (s *DeploySuite) TestDeployListByService(c *gocheck.C) {
	var result []Deploy
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	srv := service.Service{Name: "redis", Teams: []string{s.team.Name}}
	err = srv.Create()
	c.Assert(err, gocheck.IsNil)
	instance := service.ServiceInstance{
		Name:        "redis-g1",
		ServiceName: "redis",
		Apps:        []string{"g1", "qwerty"},
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	request, err := http.NewRequest("GET", "/deploys?service=redis", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	err = s.conn.Deploys().Insert(Deploy{App: "g1", Timestamp: timestamp, Duration: duration})
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Deploys().Insert(Deploy{App: "ge", Timestamp: timestamp, Duration: duration})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Deploys().RemoveAll(nil)
	err = deploysList(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.HasLen, 1)
	c.Assert(result[0].App, gocheck.Equals, "g1")
	c.Assert(result[0].Timestamp.In(time.UTC), gocheck.DeepEquals, timestamp.In(time.UTC))
	c.Assert(result[0].Duration, gocheck.DeepEquals, duration)
}

func (s *DeploySuite) TestDeployInfo(c *gocheck.C) {
	var result map[string]interface{}
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	request, err := http.NewRequest("GET", "/deploys/deploy?:deploy=53e143cb874ccb1f68000001", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	depId := bson.ObjectIdHex("53e143cb874ccb1f68000001")
	otherDepId := bson.ObjectIdHex("53e143cb874ccb1f68000002")
	timestamp := time.Now()
	duration := time.Duration(10e9)
	lastDeploy := Deploy{ID: depId, App: "g1", Timestamp: timestamp, Duration: duration, Commit: "e82nn93nd93mm12o2ueh83dhbd3iu112", Error: ""}
	err = s.conn.Deploys().Insert(lastDeploy)
	c.Assert(err, gocheck.IsNil)
	previousDeploy := Deploy{ID: otherDepId, App: "g1", Timestamp: timestamp.Add(-3600 * time.Second), Duration: duration, Commit: "e293e3e3me03ejm3puejmp3ej3iejop32", Error: ""}
	err = s.conn.Deploys().Insert(previousDeploy)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Deploys().RemoveAll(nil)
	expected := "test_diff"
	h := testHandler{content: expected}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	err = deployInfo(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	expected_deploy := map[string]interface{}{
		"Id":        depId.Hex(),
		"App":       "g1",
		"Timestamp": timestamp.Format(time.RFC3339),
		"Duration":  10.0,
		"Commit":    "e82nn93nd93mm12o2ueh83dhbd3iu112",
		"Error":     "",
		"Diff":      expected,
	}
	c.Assert(result, gocheck.DeepEquals, expected_deploy)
}
