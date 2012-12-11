// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/api/bind"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/repository"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	stdlog "log"
	"os"
	"path"
	"strings"
)

var output = `2012-06-05 17:03:36,887 WARNING ssl-hostname-verification is disabled for this environment
2012-06-05 17:03:36,887 WARNING EC2 API calls not using secure transport
2012-06-05 17:03:36,887 WARNING S3 API calls not using secure transport
2012-06-05 17:03:36,887 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated
2012-06-05 17:03:36,896 INFO Connecting to environment...
2012-06-05 17:03:37,599 INFO Connected to environment.
2012-06-05 17:03:37,727 INFO Connecting to machine 0 at 10.170.0.191
export DATABASE_HOST=localhost
export DATABASE_USER=root
export DATABASE_PASSWORD=secret`

func (s *S) TestGet(c *C) {
	dir, err := commandmocker.Add("juju", "")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	newApp := App{Name: "myApp", Framework: "Django"}
	err = db.Session.Apps().Insert(newApp)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": newApp.Name})
	newApp.Env = map[string]bind.EnvVar{}
	newApp.Logs = []Applog{}
	err = db.Session.Apps().Update(bson.M{"name": newApp.Name}, &newApp)
	c.Assert(err, IsNil)
	myApp := App{Name: "myApp"}
	err = myApp.Get()
	c.Assert(err, IsNil)
	c.Assert(myApp.Name, Equals, newApp.Name)
	c.Assert(myApp.State, Equals, newApp.State)
}

func (s *S) TestDestroy(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{
		Name:      "ritual",
		Framework: "ruby",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			{
				Name:    "duvido",
				Machine: 3,
			},
		},
	}
	err = CreateApp(&a)
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	err = a.Destroy()
	c.Assert(err, IsNil)
	err = a.Get()
	c.Assert(err, NotNil)
	logStr := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(logStr, Matches, ".*destroy-service ritual.*")
	c.Assert(logStr, Matches, ".*terminate-machine 3.*")
	qt, err := db.Session.Apps().Find(bson.M{"name": a.Name}).Count()
	c.Assert(err, IsNil)
	c.Assert(qt, Equals, 0)
}

func (s *S) TestDestroyWithoutUnits(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{
		Name:  "x4",
		Units: []Unit{{Machine: 1}},
	}
	err = CreateApp(&app)
	c.Assert(err, IsNil)
	err = app.Destroy()
	c.Assert(err, IsNil)
}

func (s *S) TestFailingDestroy(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	a := App{
		Name:      "ritual",
		Framework: "ruby",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			{
				Name:    "duvido",
				Machine: 3,
			},
		},
	}
	err = CreateApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	commandmocker.Remove(dir)
	dir, err = commandmocker.Error("juju", "juju failed", 25)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	err = a.Destroy()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Failed to destroy unit: exit status 25\njuju failed")
}

// TODO(fss): simplify this test. Right now, it's a little monster.
func (s *S) TestCreateApp(c *C) {
	patchRandomReader()
	defer unpatchRandomReader()
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	server := FakeQueueServer{}
	server.Start("127.0.0.1:0")
	defer server.Stop()
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	a := App{
		Name:      "appname",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	c.Assert(err, IsNil)
	old, err := config.Get("queue-server")
	if err != nil {
		defer config.Set("queue-server", old)
	}
	config.Set("queue-server", server.listener.Addr().String())

	err = CreateApp(&a)
	c.Assert(err, IsNil)
	defer a.Destroy()
	c.Assert(a.State, Equals, "pending")
	var retrievedApp App
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&retrievedApp)
	c.Assert(err, IsNil)
	c.Assert(retrievedApp.Name, Equals, a.Name)
	c.Assert(retrievedApp.Framework, Equals, a.Framework)
	c.Assert(retrievedApp.State, Equals, a.State)
	str := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(str, Matches, ".*deploy --repository=/home/charms local:django appname.*")
	env := a.InstanceEnv(s3InstanceName)
	c.Assert(env["TSURU_S3_ENDPOINT"].Value, Equals, s.t.S3Server.URL())
	c.Assert(env["TSURU_S3_ENDPOINT"].Public, Equals, false)
	c.Assert(env["TSURU_S3_LOCATIONCONSTRAINT"].Value, Equals, "true")
	c.Assert(env["TSURU_S3_LOCATIONCONSTRAINT"].Public, Equals, false)
	e, ok := env["TSURU_S3_ACCESS_KEY_ID"]
	c.Assert(ok, Equals, true)
	c.Assert(e.Public, Equals, false)
	e, ok = env["TSURU_S3_SECRET_KEY"]
	c.Assert(ok, Equals, true)
	c.Assert(e.Public, Equals, false)
	c.Assert(env["TSURU_S3_BUCKET"].Value, HasLen, maxBucketSize)
	c.Assert(env["TSURU_S3_BUCKET"].Value, Equals, "appnamee3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3")
	c.Assert(env["TSURU_S3_BUCKET"].Public, Equals, false)
	env = a.InstanceEnv("")
	c.Assert(env["APPNAME"].Value, Equals, a.Name)
	c.Assert(env["APPNAME"].Public, Equals, false)
	c.Assert(env["TSURU_HOST"].Value, Equals, expectedHost)
	c.Assert(env["TSURU_HOST"].Public, Equals, false)
	expectedMessage := queue.Message{
		Action: RegenerateApprc,
		Args:   []string{a.Name},
	}
	server.Lock()
	defer server.Unlock()
	c.Assert(server.messages, DeepEquals, []queue.Message{expectedMessage})
}

func (s *S) TestCantCreateTwoAppsWithTheSameName(c *C) {
	dir, err := commandmocker.Add("juju", "")
	defer commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	err = db.Session.Apps().Insert(bson.M{"name": "appName"})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": "appName"})
	a := App{Name: "appName"}
	err = CreateApp(&a)
	defer a.Destroy() // clean mess if test fail
	c.Assert(err, NotNil)
}

func (s *S) TestCantCreateAppWithInvalidName(c *C) {
	a := App{
		Name:      "1123app",
		Framework: "ruby",
	}
	err := CreateApp(&a)
	c.Assert(err, NotNil)
	e, ok := err.(*ValidationError)
	c.Assert(ok, Equals, true)
	msg := "Invalid app name, your app should have at most 63 " +
		"characters, containing only lower case letters or numbers, " +
		"starting with a letter."
	c.Assert(e.Message, Equals, msg)
}

func (s *S) TestDoesNotSaveTheAppInTheDatabaseIfJujuFail(c *C) {
	dir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{
		Name:      "theirapp",
		Framework: "ruby",
		Units:     []Unit{{Machine: 1}},
	}
	err = CreateApp(&a)
	defer a.Destroy() // clean mess if test fail
	c.Assert(err, NotNil)
	expected := `Failed to deploy: exit status 1
juju failed`
	c.Assert(err.Error(), Equals, expected)
	err = a.Get()
	c.Assert(err, NotNil)
}

func (s *S) TestDeletesIAMCredentialsAndS3BucketIfJujuFail(c *C) {
	source := patchRandomReader()
	defer unpatchRandomReader()
	dir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{
		Name:      "theirapp",
		Framework: "ruby",
		Units:     []Unit{{Machine: 1}},
	}
	err = CreateApp(&a)
	defer a.Destroy() // clean mess if test fail
	c.Assert(err, NotNil)
	iam := getIAMEndpoint()
	_, err = iam.GetUser("theirapp")
	c.Assert(err, NotNil)
	s3 := getS3Endpoint()
	bucketName := fmt.Sprintf("%s%x", a.Name, source[:(maxBucketSize-len(a.Name)/2)])
	bucket := s3.Bucket(bucketName)
	_, err = bucket.Get("")
	c.Assert(err, NotNil)
}

func (s *S) TestAppendOrUpdate(c *C) {
	dir, err := commandmocker.Add("juju", "")
	defer commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	a := App{
		Name:      "appName",
		Framework: "django",
	}
	u := Unit{Name: "someapp", Ip: "", Machine: 3, InstanceId: "i-00000zz8"}
	a.AddUnit(&u)
	c.Assert(len(a.Units), Equals, 1)
	u = Unit{Name: "someapp", Ip: "192.168.0.12", Machine: 3, InstanceId: "i-00000zz8", MachineAgentState: "running"}
	a.AddUnit(&u)
	c.Assert(len(a.Units), Equals, 1)
	c.Assert(a.Units[0], DeepEquals, u)
}

func (s *S) TestGrantAccess(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{}}
	err := a.Grant(&s.team)
	c.Assert(err, IsNil)
	_, found := a.Find(&s.team)
	c.Assert(found, Equals, true)
}

func (s *S) TestGrantAccessKeepTeamsSorted(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{"acid-rain", "zito"}}
	err := a.Grant(&s.team)
	c.Assert(err, IsNil)
	c.Assert(a.Teams, DeepEquals, []string{"acid-rain", s.team.Name, "zito"})
}

func (s *S) TestGrantAccessFailsIfTheTeamAlreadyHasAccessToTheApp(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{s.team.Name}}
	err := a.Grant(&s.team)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This team already has access to this app$")
}

func (s *S) TestRevokeAccess(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{s.team.Name}}
	err := a.Revoke(&s.team)
	c.Assert(err, IsNil)
	_, found := a.Find(&s.team)
	c.Assert(found, Equals, false)
}

func (s *S) TestRevoke(c *C) {
	a := App{Name: "test", Teams: []string{"team1", "team2", "team3", "team4"}}
	err := a.Revoke(&auth.Team{Name: "team2"})
	c.Assert(err, IsNil)
	c.Assert(a.Teams, DeepEquals, []string{"team1", "team3", "team4"})
	err = a.Revoke(&auth.Team{Name: "team4"})
	c.Assert(err, IsNil)
	c.Assert(a.Teams, DeepEquals, []string{"team1", "team3"})
	err = a.Revoke(&auth.Team{Name: "team1"})
	c.Assert(err, IsNil)
	c.Assert(a.Teams, DeepEquals, []string{"team3"})
}

func (s *S) TestRevokeAccessFailsIfTheTeamsDoesNotHaveAccessToTheApp(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{}}
	err := a.Revoke(&s.team)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This team does not have access to this app$")
}

func (s *S) TestSetEnvNewAppsTheMapIfItIsNil(c *C) {
	a := App{Name: "how-many-more-times"}
	c.Assert(a.Env, IsNil)
	env := bind.EnvVar{Name: "PATH", Value: "/"}
	a.setEnv(env)
	c.Assert(a.Env, NotNil)
}

func (s *S) TestSetEnvironmentVariableToApp(c *C) {
	a := App{Name: "appName", Framework: "django"}
	a.setEnv(bind.EnvVar{Name: "PATH", Value: "/", Public: true})
	env := a.Env["PATH"]
	c.Assert(env.Name, Equals, "PATH")
	c.Assert(env.Value, Equals, "/")
	c.Assert(env.Public, Equals, true)
}

func (s *S) TestSetEnvRespectsThePublicOnlyFlagKeepPrivateVariablesWhenItsTrue(c *C) {
	dir, err := commandmocker.Add("juju", "")
	defer commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	a := App{
		Name:  "myapp",
		Units: []Unit{{Machine: 1}},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
		},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	envs := []bind.EnvVar{
		{
			Name:   "DATABASE_HOST",
			Value:  "remotehost",
			Public: false,
		},
		{
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	err = a.SetEnvsToApp(envs, true, false)
	c.Assert(err, IsNil)
	newApp := App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	c.Assert(newApp.Env, DeepEquals, expected)
}

func (s *S) TestSetEnvRespectsThePublicOnlyFlagOverwrittenAllVariablesWhenItsFalse(c *C) {
	dir, err := commandmocker.Add("juju", "")
	defer commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	a := App{
		Name: "myapp",
		Units: []Unit{
			{Machine: 1},
		},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
		},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	envs := []bind.EnvVar{
		{
			Name:   "DATABASE_HOST",
			Value:  "remotehost",
			Public: true,
		},
		{
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	err = a.SetEnvsToApp(envs, false, false)
	c.Assert(err, IsNil)
	newApp := App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "remotehost",
			Public: true,
		},
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	c.Assert(newApp.Env, DeepEquals, expected)
}

func (s *S) TestUnsetEnvRespectsThePublicOnlyFlagKeepPrivateVariablesWhenItsTrue(c *C) {
	dir, err := commandmocker.Add("juju", "")
	defer commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	a := App{
		Name: "myapp",
		Units: []Unit{
			{Machine: 1},
		},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
			"DATABASE_PASSWORD": {
				Name:   "DATABASE_PASSWORD",
				Value:  "123",
				Public: true,
			},
		},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = a.UnsetEnvsFromApp([]string{"DATABASE_HOST", "DATABASE_PASSWORD"}, true, false)
	c.Assert(err, IsNil)
	newApp := App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
	}
	c.Assert(newApp.Env, DeepEquals, expected)
}

func (s *S) TestUnsetEnvRespectsThePublicOnlyFlagUnsettingAllVariablesWhenItsFalse(c *C) {
	dir, err := commandmocker.Add("juju", "")
	defer commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	a := App{
		Name: "myapp",
		Units: []Unit{
			{Machine: 1},
		},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
			"DATABASE_PASSWORD": {
				Name:   "DATABASE_PASSWORD",
				Value:  "123",
				Public: true,
			},
		},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = a.UnsetEnvsFromApp([]string{"DATABASE_HOST", "DATABASE_PASSWORD"}, false, false)
	c.Assert(err, IsNil)
	newApp := App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, IsNil)
	c.Assert(newApp.Env, DeepEquals, map[string]bind.EnvVar{})
}

func (s *S) TestGetEnvironmentVariableFromApp(c *C) {
	a := App{Name: "whole-lotta-love"}
	a.setEnv(bind.EnvVar{Name: "PATH", Value: "/"})
	v, err := a.getEnv("PATH")
	c.Assert(err, IsNil)
	c.Assert(v.Value, Equals, "/")
}

func (s *S) TestGetEnvReturnsErrorIfTheVariableIsNotDeclared(c *C) {
	a := App{Name: "what-is-and-what-should-never"}
	a.Env = make(map[string]bind.EnvVar)
	_, err := a.getEnv("PATH")
	c.Assert(err, NotNil)
}

func (s *S) TestGetEnvReturnsErrorIfTheEnvironmentMapIsNil(c *C) {
	a := App{Name: "what-is-and-what-should-never"}
	_, err := a.getEnv("PATH")
	c.Assert(err, NotNil)
}

func (s *S) TestInstanceEnvironmentReturnEnvironmentVariablesForTheServer(c *C) {
	envs := map[string]bind.EnvVar{
		"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "localhost", Public: false, InstanceName: "mysql"},
		"DATABASE_USER": {Name: "DATABASE_USER", Value: "root", Public: true, InstanceName: "mysql"},
		"HOST":          {Name: "HOST", Value: "10.0.2.1", Public: false, InstanceName: "redis"},
	}
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "localhost", Public: false, InstanceName: "mysql"},
		"DATABASE_USER": {Name: "DATABASE_USER", Value: "root", Public: true, InstanceName: "mysql"},
	}
	a := App{Name: "hi-there", Env: envs}
	c.Assert(a.InstanceEnv("mysql"), DeepEquals, expected)
}

func (s *S) TestInstanceEnvironmentDoesNotPanicIfTheEnvMapIsNil(c *C) {
	a := App{Name: "hi-there"}
	c.Assert(a.InstanceEnv("mysql"), DeepEquals, map[string]bind.EnvVar{})
}

func (s *S) TestIsValid(c *C) {
	var data = []struct {
		name     string
		expected bool
	}{
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyapp", false},
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyap", false},
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmya", true},
		{"myApp", false},
		{"my app", false},
		{"123myapp", false},
		{"myapp", true},
		{"_theirapp", false},
		{"my-app", false},
		{"my_app", false},
		{"b", true},
	}
	for _, d := range data {
		a := App{Name: d.name}
		if valid := a.isValid(); valid != d.expected {
			c.Errorf("Is %q a valid app name? Expected: %v. Got: %v.", d.name, d.expected, valid)
		}
	}
}

func (s *S) TestUnit(c *C) {
	u := Unit{Name: "someapp/0", Type: "django", Machine: 10}
	a := App{Name: "appName", Framework: "django", Units: []Unit{u}}
	u2 := a.Unit()
	u.app = &a
	c.Assert(*u2, DeepEquals, u)
}

func (s *S) TestEmptyUnit(c *C) {
	a := App{Name: "myApp"}
	expected := Unit{app: &a}
	unit := a.Unit()
	c.Assert(*unit, DeepEquals, expected)
}

func (s *S) TestDeployHookAbsPath(c *C) {
	pwd, err := os.Getwd()
	c.Assert(err, IsNil)
	old, err := config.Get("git:unit-repo")
	c.Assert(err, IsNil)
	config.Set("git:unit-repo", pwd)
	defer config.Set("git:unit-repo", old)
	expected := path.Join(pwd, "testdata", "pre.sh")
	command := "testdata/pre.sh"
	got, err := deployHookAbsPath(command)
	c.Assert(err, IsNil)
	c.Assert(got, Equals, expected)
}

func (s *S) TestDeployHookAbsPathAbsoluteCommands(c *C) {
	command := "python manage.py syncdb --noinput"
	expected := "python manage.py syncdb --noinput"
	got, err := deployHookAbsPath(command)
	c.Assert(err, IsNil)
	c.Assert(got, Equals, expected)
}

func (s *S) TestLoadHooks(c *C) {
	output := `pre-restart:
  - testdata/pre.sh
pos-restart:
  - testdata/pos.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	err = a.loadHooks()
	c.Assert(err, IsNil)
	c.Assert(a.hooks.PreRestart, DeepEquals, []string{"testdata/pre.sh"})
	c.Assert(a.hooks.PosRestart, DeepEquals, []string{"testdata/pos.sh"})
}

func (s *S) TestLoadHooksFiltersOutputFromJuju(c *C) {
	output := `2012-06-05 17:26:15,881 WARNING ssl-hostname-verification is disabled for this environment
2012-06-05 17:26:15,881 WARNING EC2 API calls not using secure transport
2012-06-05 17:26:15,881 WARNING S3 API calls not using secure transport
2012-06-05 17:26:15,881 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated
2012-06-05 17:26:15,891 INFO Connecting to environment...
2012-06-05 17:26:16,657 INFO Connected to environment.
2012-06-05 17:26:16,860 INFO Connecting to machine 0 at 10.170.0.191
pre-restart:
  - testdata/pre.sh
  - ls -lh
  - sudo rm -rf /
pos-restart:
  - testdata/pos.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	err = a.loadHooks()
	c.Assert(err, IsNil)
	c.Assert(a.hooks.PreRestart, DeepEquals, []string{"testdata/pre.sh", "ls -lh", "sudo rm -rf /"})
	c.Assert(a.hooks.PosRestart, DeepEquals, []string{"testdata/pos.sh"})
}

func (s *S) TestLoadHooksWithListOfCommands(c *C) {
	output := `pre-restart:
  - testdata/pre.sh
  - ls -lh
  - sudo rm -rf /
pos-restart:
  - testdata/pos.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	err = a.loadHooks()
	c.Assert(err, IsNil)
	c.Assert(a.hooks.PreRestart, DeepEquals, []string{"testdata/pre.sh", "ls -lh", "sudo rm -rf /"})
	c.Assert(a.hooks.PosRestart, DeepEquals, []string{"testdata/pos.sh"})
}

func (s *S) TestLoadHooksWithError(c *C) {
	dir, err := commandmocker.Error("juju", "something", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{Name: "something", Framework: "django"}
	err = a.loadHooks()
	c.Assert(err, IsNil)
	c.Assert(a.hooks.PreRestart, IsNil)
	c.Assert(a.hooks.PosRestart, IsNil)
}

func (s *S) TestPreRestart(c *C) {
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
		hooks: &conf{
			PreRestart: []string{"pre.sh"},
			PosRestart: []string{"pos.sh"},
		},
	}
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	w := new(bytes.Buffer)
	err = a.preRestart(w)
	c.Assert(err, IsNil)
	c.Assert(err, IsNil)
	st := strings.Replace(w.String(), "\n", "###", -1)
	c.Assert(st, Matches, `.*\[ -f /home/application/apprc \] && source /home/application/apprc; \[ -d /home/application/current \] && cd /home/application/current;.*pre.sh$`)
}

func (s *S) TestPreRestartWhenAppConfDoesNotExists(c *C) {
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{Name: "something", Framework: "django"}
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	err = a.preRestart(w)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	regexp := ".*Skipping pre-restart hooks..."
	c.Assert(st[len(st)-2], Matches, regexp)
}

func (s *S) TestSkipsPreRestartWhenPreRestartSectionDoesNotExists(c *C) {
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
		hooks: &conf{
			PosRestart: []string{"somescript.sh"},
		},
	}
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	err := a.preRestart(w)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	c.Assert(st[len(st)-2], Matches, ".*Skipping pre-restart hooks...")
}

func (s *S) TestPosRestart(c *C) {
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
		hooks: &conf{
			PosRestart: []string{"pos.sh"},
		},
	}
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	w := new(bytes.Buffer)
	err = a.posRestart(w)
	c.Assert(err, IsNil)
	st := strings.Replace(w.String(), "\n", "###", -1)
	c.Assert(st, Matches, `.*\[ -f /home/application/apprc \] && source /home/application/apprc; \[ -d /home/application/current \] && cd /home/application/current;.*pos.sh$`)
}

func (s *S) TestPosRestartWhenAppConfDoesNotExists(c *C) {
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{Name: "something", Framework: "django"}
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	err = a.posRestart(w)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	c.Assert(st[len(st)-2], Matches, ".*Skipping pos-restart hooks...")
}

func (s *S) TestSkipsPosRestartWhenPosRestartSectionDoesNotExists(c *C) {
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
		hooks: &conf{
			PreRestart: []string{"somescript.sh"},
		},
	}
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	err := a.posRestart(w)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	c.Assert(st[len(st)-2], Matches, ".*Skipping pos-restart hooks...")
}

func (s *S) TestInstallDeps(c *C) {
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	a := App{
		Name:      "someApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			{
				AgentState:        "started",
				MachineAgentState: "running",
				InstanceState:     "running",
				Machine:           4,
			},
		},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	var buf bytes.Buffer
	err = InstallDeps(&a, &buf)
	c.Assert(err, IsNil)
	c.Assert(buf.String(), Equals, "ssh -o StrictHostKeyChecking no -q 4 /var/lib/tsuru/hooks/dependencies")
}

func (s *S) TestInstallDepsWithCustomStdout(c *C) {
	a := App{
		Name:      "someApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			{
				AgentState:        "started",
				MachineAgentState: "running",
				InstanceState:     "running",
				Machine:           4,
			},
		},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	var b bytes.Buffer
	err = InstallDeps(&a, &b)
	c.Assert(err, IsNil)
	c.Assert(b.String(), Matches, `.* /var/lib/tsuru/hooks/dependencies`)
}

func (s *S) TestInstallDepsWithCustomStderr(c *C) {
	a := App{
		Name:      "someApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			{
				AgentState:        "started",
				MachineAgentState: "running",
				InstanceState:     "running",
				Machine:           4,
			},
		},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	tmpdir, err := commandmocker.Error("juju", "$*", 42)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	var b bytes.Buffer
	err = InstallDeps(&a, &b)
	c.Assert(err, NotNil)
	c.Assert(b.String(), Matches, `.* /var/lib/tsuru/hooks/dependencies`)
}

func (s *S) TestRestart(c *C) {
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	a := App{
		Name:      "someApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			{
				AgentState:        "started",
				MachineAgentState: "running",
				InstanceState:     "running",
				Machine:           4,
			},
		},
	}
	var b bytes.Buffer
	err = Restart(&a, &b)
	c.Assert(err, IsNil)
	result := strings.Replace(b.String(), "\n", "#", -1)
	c.Assert(result, Matches, ".*/var/lib/tsuru/hooks/restart.*")
	c.Assert(result, Matches, ".*# ---> Restarting your app#.*")
}

func (s *S) TestRestartRunPreRestartHook(c *C) {
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	a := App{
		Name:      "someApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			{
				AgentState:        "started",
				MachineAgentState: "running",
				InstanceState:     "running",
				Machine:           4,
			},
		},
		hooks: &conf{
			PreRestart: []string{"pre.sh"},
		},
	}
	var buf bytes.Buffer
	err = Restart(&a, &buf)
	c.Assert(err, IsNil)
	content := buf.String()
	content = strings.Replace(content, "\n", "###", -1)
	c.Assert(content, Matches, "^.*### ---> Running pre-restart###.*$")
}

func (s *S) TestRestartRunsPosRestartHook(c *C) {
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	a := App{
		Name:      "someApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			{
				AgentState:        "started",
				MachineAgentState: "running",
				InstanceState:     "running",
				Machine:           4,
			},
		},
		hooks: &conf{
			PosRestart: []string{"pos.sh"},
		},
	}
	var buf bytes.Buffer
	err = Restart(&a, &buf)
	c.Assert(err, IsNil)
	content := buf.String()
	content = strings.Replace(content, "\n", "###", -1)
	c.Assert(content, Matches, "^.*### ---> Running pos-restart###.*$")
}

func (s *S) TestLog(c *C) {
	dir, err := commandmocker.Add("juju", "")
	defer commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	a := App{Name: "newApp"}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = a.Log("last log msg", "tsuru")
	c.Assert(err, IsNil)
	var instance App
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	logLen := len(instance.Logs)
	c.Assert(instance.Logs[logLen-1].Message, Equals, "last log msg")
	c.Assert(instance.Logs[logLen-1].Source, Equals, "tsuru")
}

func (s *S) TestLogShouldAddOneRecordByLine(c *C) {
	a := App{Name: "newApp"}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = a.Log("last log msg\nfirst log", "source")
	c.Assert(err, IsNil)
	instance := App{}
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	logLen := len(instance.Logs)
	c.Assert(instance.Logs[logLen-2].Message, Equals, "last log msg")
	c.Assert(instance.Logs[logLen-1].Message, Equals, "first log")
}

func (s *S) TestLogShouldNotLogWhiteLines(c *C) {
	a := App{
		Name: "newApp",
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = a.Log("some message", "tsuru")
	c.Assert(err, IsNil)
	err = a.Log("", "")
	c.Assert(err, IsNil)
	var instance App
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	logLen := len(instance.Logs)
	c.Assert(instance.Logs[logLen-1].Message, Not(Equals), "")
}

func (s *S) TestLogShouldNotLogWarnings(c *C) {
	a := App{
		Name: "newApp",
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = a.Log("some message", "tsuru")
	c.Assert(err, IsNil)
	badLogs := []string{
		"2012-11-28 16:00:35,615 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated",
		"2012-11-28 16:00:35,616 INFO Connecting to environment...",
		"/usr/local/lib/python2.7/dist-packages/txAWS-0.2.3-py2.7.egg/txaws/client/base.py:208: UserWarning: The client attribute on BaseQuery is deprecated and will go away in future release.",
		"warnings.warn('The client attribute on BaseQuery is deprecated and'",
	}
	for _, log := range badLogs {
		err = a.Log(log, "")
		c.Assert(err, IsNil)
	}
	for _, log := range badLogs {
		lenght, err := db.Session.Apps().Find(bson.M{"logs.message": log}).Count()
		c.Assert(err, IsNil)
		c.Assert(lenght, Equals, 0)
	}

	var instance App
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&instance)
}

func (s *S) TestGetTeams(c *C) {
	app := App{Name: "app", Teams: []string{s.team.Name}}
	teams := app.teams()
	c.Assert(teams, HasLen, 1)
	c.Assert(teams[0].Name, Equals, s.team.Name)
}

func (s *S) TestSetTeams(c *C) {
	app := App{Name: "app"}
	app.SetTeams([]auth.Team{s.team})
	c.Assert(app.Teams, DeepEquals, []string{s.team.Name})
}

func (s *S) TestSetTeamsSortTeamNames(c *C) {
	app := App{Name: "app"}
	app.SetTeams([]auth.Team{s.team, {Name: "zzz"}, {Name: "aaa"}})
	c.Assert(app.Teams, DeepEquals, []string{"aaa", s.team.Name, "zzz"})
}

func (s *S) TestGetUnits(c *C) {
	app := App{Units: []Unit{{Ip: "1.1.1.1"}}}
	expected := []bind.Unit{bind.Unit(&Unit{Ip: "1.1.1.1", app: &app})}
	c.Assert(app.GetUnits(), DeepEquals, expected)
}

func (s *S) TestDeployShouldCallJujuDeployCommand(c *C) {
	a := App{
		Name:      "smashed_pumpkin",
		Framework: "golang",
	}
	err := db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	err = a.deploy()
	c.Assert(err, IsNil)
	logged := strings.Replace(w.String(), "\n", " ", -1)
	expected := ".*deploying golang with name smashed_pumpkin.*"
	c.Assert(logged, Matches, expected)
	expected = ".*deploy --repository=/home/charms local:golang smashed_pumpkin.*"
	c.Assert(logged, Matches, expected)
}

func (s *S) TestAppMarshalJson(c *C) {
	app := App{
		Name:      "Name",
		State:     "State",
		Framework: "Framework",
		Teams:     []string{"team1"},
	}
	expected := make(map[string]interface{})
	expected["Name"] = "Name"
	expected["State"] = "State"
	expected["Framework"] = "Framework"
	expected["Repository"] = repository.GetUrl(app.Name)
	expected["Teams"] = []interface{}{"team1"}
	expected["Units"] = interface{}(nil)
	data, err := app.MarshalJSON()
	c.Assert(err, IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, IsNil)
	c.Assert(result, DeepEquals, expected)
}

func (s *S) TestRun(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{
		Name: "myapp",
		Units: []Unit{
			{
				Name:              "someapp/0",
				Type:              "django",
				Machine:           10,
				AgentState:        "started",
				MachineAgentState: "running",
				InstanceState:     "running",
			},
		},
	}
	var buf bytes.Buffer
	err = app.Run("ls -lh", &buf)
	c.Assert(err, IsNil)
	c.Assert(buf.String(), Equals, "ssh -o StrictHostKeyChecking no -q 10 [ -f /home/application/apprc ] && source /home/application/apprc; [ -d /home/application/current ] && cd /home/application/current; ls -lh")
}

func (s *S) TestSerializeEnvVars(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{
		Name:  "time",
		Teams: []string{s.team.Name},
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running", Machine: 1},
		},
		Env: map[string]bind.EnvVar{
			"http_proxy": {
				Name:   "http_proxy",
				Value:  "http://theirproxy.com:3128/",
				Public: true,
			},
		},
	}
	app.SerializeEnvVars()
	c.Assert(commandmocker.Ran(dir), Equals, true)
	output := strings.Replace(commandmocker.Output(dir), "\n", " ", -1)
	outputRegexp := `^.*1 cat > /home/application/apprc <<END # generated by tsuru .*`
	outputRegexp += ` export http_proxy="http://theirproxy.com:3128/" END $`
	c.Assert(output, Matches, outputRegexp)
}

func (s *S) TestListReturnsAppsForAGivenUser(c *C) {
	a := App{
		Name:  "testapp",
		Teams: []string{s.team.Name},
	}
	a2 := App{
		Name:  "othertestapp",
		Teams: []string{"commonteam", s.team.Name},
	}
	err := db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	err = db.Session.Apps().Insert(&a2)
	c.Assert(err, IsNil)
	defer func() {
		db.Session.Apps().Remove(bson.M{"name": a.Name})
		db.Session.Apps().Remove(bson.M{"name": a2.Name})
	}()
	apps, err := List(s.user)
	c.Assert(err, IsNil)
	c.Assert(len(apps), Equals, 2)
}

func (s *S) TestListReturnsEmptyAppArrayWhenUserHasNoAccessToAnyApp(c *C) {
	apps, err := List(s.user)
	c.Assert(err, IsNil)
	c.Assert(apps, DeepEquals, []App(nil))
}

func (s *S) TestListReturnsAllAppsWhenUserIsInAdminTeam(c *C) {
	a := App{Name: "testApp", Teams: []string{"notAdmin", "noSuperUser"}}
	err := db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	s.createAdminUserAndTeam(c)
	defer s.removeAdminUserAndTeam(c)
	apps, err := List(s.admin)
	c.Assert(len(apps), Greater, 0)
	c.Assert(apps[0].Name, Equals, "testApp")
	c.Assert(apps[0].Teams, DeepEquals, []string{"notAdmin", "noSuperUser"})
}
