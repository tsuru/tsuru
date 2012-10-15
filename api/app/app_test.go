// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/api/bind"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/repository"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	stdlog "log"
	"os"
	"path"
	"strings"
)

func (s *S) TestGet(c *C) {
	newApp := App{Name: "myApp", Framework: "Django"}
	err := createApp(&newApp)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": newApp.Name})
	newApp.Env = map[string]bind.EnvVar{}
	newApp.Logs = []applog{}
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
	err = createApp(&a)
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	err = a.destroy()
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

func (s *S) TestCreateApp(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	a := App{
		Name:      "appName",
		Framework: "django",
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	c.Assert(a.State, Equals, "pending")
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	var retrievedApp App
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&retrievedApp)
	c.Assert(err, IsNil)
	c.Assert(retrievedApp.Name, Equals, a.Name)
	c.Assert(retrievedApp.Framework, Equals, a.Framework)
	c.Assert(retrievedApp.State, Equals, a.State)
	str := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(str, Matches, ".*deploy --repository=/home/charms local:django appName.*")
}

func (s *S) TestCantCreateTwoAppsWithTheSameName(c *C) {
	err := db.Session.Apps().Insert(bson.M{"name": "appName"})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": "appName"})
	a := App{Name: "appName"}
	err = createApp(&a)
	c.Assert(err, NotNil)
}

func (s *S) TestDoesNotSaveTheAppInTheDatabaseIfJujuFail(c *C) {
	dir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{
		Name:      "myapp",
		Framework: "ruby",
	}
	err = createApp(&a)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^.*juju failed.*$")
	err = a.Get()
	c.Assert(err, NotNil)
}

func (s *S) TestAppendOrUpdate(c *C) {
	a := App{
		Name:      "appName",
		Framework: "django",
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
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
	err := a.grant(&s.team)
	c.Assert(err, IsNil)
	_, found := a.find(&s.team)
	c.Assert(found, Equals, true)
}

func (s *S) TestGrantAccessKeepTeamsSorted(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{"acid-rain", "zito"}}
	err := a.grant(&s.team)
	c.Assert(err, IsNil)
	c.Assert(a.Teams, DeepEquals, []string{"acid-rain", s.team.Name, "zito"})
}

func (s *S) TestGrantAccessFailsIfTheTeamAlreadyHasAccessToTheApp(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{s.team.Name}}
	err := a.grant(&s.team)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This team already has access to this app$")
}

func (s *S) TestRevokeAccess(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{s.team.Name}}
	err := a.revoke(&s.team)
	c.Assert(err, IsNil)
	_, found := a.find(&s.team)
	c.Assert(found, Equals, false)
}

func (s *S) TestRevoke(c *C) {
	a := App{Name: "test", Teams: []string{"team1", "team2", "team3", "team4"}}
	err := a.revoke(&auth.Team{Name: "team2"})
	c.Assert(err, IsNil)
	c.Assert(a.Teams, DeepEquals, []string{"team1", "team3", "team4"})
	err = a.revoke(&auth.Team{Name: "team4"})
	c.Assert(err, IsNil)
	c.Assert(a.Teams, DeepEquals, []string{"team1", "team3"})
	err = a.revoke(&auth.Team{Name: "team1"})
	c.Assert(err, IsNil)
	c.Assert(a.Teams, DeepEquals, []string{"team3"})
}

func (s *S) TestRevokeAccessFailsIfTheTeamsDoesNotHaveAccessToTheApp(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{}}
	err := a.revoke(&s.team)
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

func (s *S) TestUnit(c *C) {
	u := Unit{Name: "someapp/0", Type: "django", Machine: 10}
	a := App{Name: "appName", Framework: "django", Units: []Unit{u}}
	u2 := a.unit()
	u.app = &a
	c.Assert(*u2, DeepEquals, u)
}

func (s *S) TestEmptyUnit(c *C) {
	a := App{Name: "myApp"}
	expected := Unit{app: &a}
	unit := a.unit()
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

func (s *S) TestAppConf(c *C) {
	output := `
something that must be discarded
another thing that must also be discarded
one more
========
pre-restart:
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
	conf, err := a.conf()
	c.Assert(err, IsNil)
	c.Assert(conf.PreRestart, DeepEquals, []string{"testdata/pre.sh"})
	c.Assert(conf.PosRestart, DeepEquals, []string{"testdata/pos.sh"})
}

func (s *S) TestAppConfWithListOfCommands(c *C) {
	output := `
trash
========
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
	conf, err := a.conf()
	c.Assert(err, IsNil)
	c.Assert(conf.PreRestart, DeepEquals, []string{"testdata/pre.sh", "ls -lh", "sudo rm -rf /"})
	c.Assert(conf.PosRestart, DeepEquals, []string{"testdata/pos.sh"})
}

func (s *S) TestAppConfWhenFileDoesNotExists(c *C) {
	output := `
something that must be discarded
another thing that must also be discarded
one more
========
File or directory does not exists
$(exit 1)
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{Name: "something", Framework: "django"}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	c.Assert(conf.PreRestart, IsNil)
	c.Assert(conf.PosRestart, IsNil)
}

func (s *S) TestPreRestart(c *C) {
	output := `
something that must be discarded
another thing that must also be discarded
one more
========
pre-restart:
  - pre.sh
pos-restart:
  - pos.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	dir, err = commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	out, err := a.preRestart(conf)
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	c.Assert(st[len(st)-2], Matches, `.*\[ -f /home/application/apprc \] && source /home/application/apprc; \[ -d /home/application/current \] && cd /home/application/current; /bin/bash -c.*pre.sh$`)
	c.Assert(string(out), Matches, `.*\[ -f /home/application/apprc \] && source /home/application/apprc; \[ -d /home/application/current \] && cd /home/application/current; /bin/bash -c.*pre.sh$`)
}

func (s *S) TestPreRestartWhenAppConfDoesNotExists(c *C) {
	output := `
something that must be discarded
another thing that must also be discarded
one more
========
File or directory does not exists
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{Name: "something", Framework: "django"}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	_, err = a.preRestart(conf)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	regexp := ".*app.conf file does not exists or is in the right place. Skipping pre-restart hook..."
	c.Assert(st[len(st)-2], Matches, regexp)
}

func (s *S) TestSkipsPreRestartWhenPreRestartSectionDoesNotExists(c *C) {
	output := `
something that must be discarded
another thing that must also be discarded
one more
========
pos-restart:
  - somescript.sh
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
	conf, err := a.conf()
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	_, err = a.preRestart(conf)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	regexp := ".*pre-restart hook section in app conf does not exists... Skipping pre-restart hook..."
	c.Assert(st[len(st)-2], Matches, regexp)
}

func (s *S) TestPosRestart(c *C) {
	output := `
sooooome
========
pos-restart:
  - pos.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	dir, err = commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	out, err := a.posRestart(conf)
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	st := strings.Split(w.String(), "\n")
	regexp := `.*\[ -f /home/application/apprc \] && source /home/application/apprc; \[ -d /home/application/current \] && cd /home/application/current; /bin/bash -c.*pos.sh$`
	c.Assert(st[len(st)-2], Matches, regexp)
	c.Assert(string(out), Matches, regexp)
}

func (s *S) TestPosRestartWhenAppConfDoesNotExists(c *C) {
	output := `
something that must be discarded
another thing that must also be discarded
one more
========
File or directory does not exists
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{Name: "something", Framework: "django"}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	_, err = a.posRestart(conf)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	regexp := ".*app.conf file does not exists or is in the right place. Skipping pos-restart hook..."
	c.Assert(st[len(st)-2], Matches, regexp)
}

func (s *S) TestSkipsPosRestartWhenPosRestartSectionDoesNotExists(c *C) {
	output := `
something that must be discarded
another thing that must also be discarded
one more
========
pre-restart:
  - somescript.sh
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
	conf, err := a.conf()
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	_, err = a.posRestart(conf)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	regexp := ".*pos-restart hook section in app conf does not exists... Skipping pos-restart hook..."
	c.Assert(st[len(st)-2], Matches, regexp)
}

func (s *S) TestHasRestartHooksWithNoHooks(c *C) {
	output := `
something that must be discarded
========
nothing here
`
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	conf, err := a.conf()
	commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	b := a.hasRestartHooks(conf)
	c.Assert(b, Equals, false)
}

func (s *S) TestHasRestartHooksWithOneHooks(c *C) {
	output := `
something that must be discarded
========
pos-restart:
  - somefile.sh
`
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	conf, err := a.conf()
	commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	b := a.hasRestartHooks(conf)
	c.Assert(b, Equals, true)
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
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	out, err := installDeps(&a, nil, nil)
	c.Assert(err, IsNil)
	c.Assert(string(out), Equals, "ssh -o StrictHostKeyChecking no -q 4 /var/lib/tsuru/hooks/dependencies")
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
	err := createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	var b bytes.Buffer
	_, err = installDeps(&a, &b, nil)
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
	err := createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	tmpdir, err := commandmocker.Error("juju", "$*", 42)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	var b bytes.Buffer
	_, err = installDeps(&a, nil, &b)
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
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	out, err := restart(&a)
	c.Assert(err, IsNil)
	c.Assert(string(out), Equals, "ssh -o StrictHostKeyChecking no -q 4 /var/lib/tsuru/hooks/restart")
}

func (s *S) TestLogShouldStoreLog(c *C) {
	a := App{Name: "newApp"}
	err := createApp(&a)
	c.Assert(err, IsNil)
	err = a.log("last log msg")
	c.Assert(err, IsNil)
	instance := App{}
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	logLen := len(instance.Logs)
	c.Assert(instance.Logs[logLen-1].Message, Equals, "last log msg")
}

func (s *S) TestGetTeams(c *C) {
	app := App{Name: "app", Teams: []string{s.team.Name}}
	teams := app.teams()
	c.Assert(teams, HasLen, 1)
	c.Assert(teams[0].Name, Equals, s.team.Name)
}

func (s *S) TestSetTeams(c *C) {
	app := App{Name: "app"}
	app.setTeams([]auth.Team{s.team})
	c.Assert(app.Teams, DeepEquals, []string{s.team.Name})
}

func (s *S) TestSetTeamsSortTeamNames(c *C) {
	app := App{Name: "app"}
	app.setTeams([]auth.Team{s.team, {Name: "zzz"}, {Name: "aaa"}})
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
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	err = deploy(&a)
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
