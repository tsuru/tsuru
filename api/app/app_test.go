package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/bind"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	"github.com/timeredbull/tsuru/repository"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	stdlog "log"
	"strings"
)

func (s *S) TestGet(c *C) {
	newApp := App{Name: "myApp", Framework: "Django", ec2Auth: &fakeAuthorizer{}}
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
	u := Unit{Name: "duvido", Machine: 3}
	authorizer := &fakeAuthorizer{}
	a := App{
		Name:      "duvido",
		Framework: "django",
		Units:     []Unit{u},
		EC2Creds:  map[string]string{"access": "access-key-here", "secret": "secret-key-here"},
		ec2Auth:   authorizer,
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	err = a.destroy()
	c.Assert(err, IsNil)
	qtd, err := db.Session.Apps().Find(bson.M{"name": a.Name}).Count()
	c.Assert(qtd, Equals, 0)
	c.Assert(authorizer.actions, DeepEquals, []string{"setCreds access-key-here secret-key-here", "authorize " + a.Name})
}

func (s *S) TestDestroyWithMultiTenancyOnCallsJujuDestroyEnvironment(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{Name: "duvido", Machine: 3}
	a := App{
		Name:      "duvido",
		Framework: "django",
		Units:     []Unit{u},
		EC2Creds:  map[string]string{"access": "91232f6796b54ca2a2b87ef50548b123"},
		ec2Auth:   &fakeAuthorizer{},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = a.destroy()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(dir), Equals, true)
}

func (s *S) TestDestroyWithnMultiTenancyOnDoesNotDeleteTheAppIfTheDestroyEnvironmentFail(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{Name: "duvido", Machine: 3}
	a := App{
		Name:      "duvido",
		Framework: "django",
		Units:     []Unit{u},
		ec2Auth:   &fakeAuthorizer{},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	dir, err = commandmocker.Error("juju", "juju failed to destroy the environment", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	err = a.destroy()
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, ".*juju failed to destroy the environment.*")
	err = a.Get()
	c.Assert(err, IsNil)
}

func (s *S) TestDestroyWithMultiTenancyOff(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	config.Set("multi-tenant", false)
	defer config.Set("multi-tenant", true)
	a := App{
		Name:      "ritual",
		Framework: "ruby",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			Unit{
				Name:    "duvido",
				Machine: 3,
			},
		},
		ec2Auth: &fakeAuthorizer{},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	err = a.destroy()
	c.Assert(err, IsNil)
	err = a.Get()
	c.Assert(err, NotNil)
	logStr := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(logStr, Matches, ".*destroy-service -e [a-z]+ ritual.*")
	c.Assert(logStr, Matches, ".*terminate-machine -e [a-z]+ 3.*")
}

func (s *S) TestDestroyWithMultiTenancyOffDoesNotDeleteTheAppIfJujuFailToDestroyTheService(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	config.Set("multi-tenant", false)
	defer config.Set("multi-tenant", true)
	a := App{
		Name:      "ritual",
		Framework: "ruby",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			Unit{Name: "duvido", Machine: 3},
		},
		ec2Auth: &fakeAuthorizer{},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	dir, err = commandmocker.Error("juju", "juju failed to destroy the service", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	err = a.destroy()
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, ".*juju failed to destroy the service.*")
	err = a.Get()
	c.Assert(err, IsNil)
}

func (s *S) TestCreateApp(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	authorizer := fakeAuthorizer{}
	a := App{
		Name:      "appName",
		Framework: "django",
		EC2Creds:  map[string]string{"access": "access-key-here", "secret": "secret-key-here"},
		ec2Auth:   &authorizer,
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
	c.Assert(retrievedApp.JujuEnv, Equals, a.Name)
	str := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(str, Matches, ".*bootstraping juju environment appName for the app appName.*")
	c.Assert(str, Matches, ".*deploy -e appName --repository=/home/charms local:django appName.*")
	c.Assert(authorizer.actions, DeepEquals, []string{"setCreds access-key-here secret-key-here", "authorize appName"})
}

func (s *S) TestCantNewAppTwoAppsWithTheSameName(c *C) {
	err := db.Session.Apps().Insert(bson.M{"name": "appName"})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": "appName"})
	a := App{Name: "appName", ec2Auth: &fakeAuthorizer{}}
	err = createApp(&a)
	c.Assert(err, NotNil)
}

// Issue 116
func (s *S) TestDoesNotSaveTheAppInTheDatabaseIfJujuFail(c *C) {
	dir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{
		Name:      "myapp",
		Framework: "ruby",
		ec2Auth:   &fakeAuthorizer{},
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
		ec2Auth:   &fakeAuthorizer{},
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
		"DATABASE_HOST": bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: false, InstanceName: "mysql"},
		"DATABASE_USER": bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: true, InstanceName: "mysql"},
		"HOST":          bind.EnvVar{Name: "HOST", Value: "10.0.2.1", Public: false, InstanceName: "redis"},
	}
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: false, InstanceName: "mysql"},
		"DATABASE_USER": bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: true, InstanceName: "mysql"},
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
	path := "deploy/pre.sh"
	expected := "/home/application/current/deploy/pre.sh"
	got, err := deployHookAbsPath(path)
	c.Assert(err, IsNil)
	c.Assert(got, Equals, expected)
}

func (s *S) TestAppConf(c *C) {
	output := `
something that must be discarded
another thing that must also be discarded
one more
========
pre-restart: testdata/pre.sh
pos-restart: testdata/pos.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			Unit{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	c.Assert(conf.PreRestart, Equals, "testdata/pre.sh")
	c.Assert(conf.PosRestart, Equals, "testdata/pos.sh")
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
	c.Assert(conf.PreRestart, Equals, "")
	c.Assert(conf.PosRestart, Equals, "")
}

func (s *S) TestPreRestart(c *C) {
	output := `
something that must be discarded
another thing that must also be discarded
one more
========
pre-restart: pre.sh
pos-restart: pos.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			Unit{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	dir, err = commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	out, err := a.preRestart(conf)
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	c.Assert(st[len(st)-2], Matches, ".*/bin/bash /home/application/current/pre.sh$")
	c.Assert(string(out), Matches, ".*/bin/bash /home/application/current/pre.sh$")
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
	log.Target = l
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
    somescript.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			Unit{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
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
    pos.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			Unit{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	dir, err = commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	out, err := a.posRestart(conf)
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	st := strings.Split(w.String(), "\n")
	regexp := ".*/bin/bash /home/application/current/pos.sh$"
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
	log.Target = l
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
pre-restart: somescript.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			Unit{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
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
			Unit{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
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
    somefile.sh
`
	a := App{
		Name:      "something",
		Framework: "django",
		Units: []Unit{
			Unit{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
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

func (s *S) TestUpdateHooks(c *C) {
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	a := App{
		Name:      "someApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			Unit{
				AgentState:        "started",
				MachineAgentState: "running",
				InstanceState:     "running",
				Machine:           4,
			},
		},
		ec2Auth: &fakeAuthorizer{},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	out, err := a.updateHooks()
	c.Assert(err, IsNil)
	c.Assert(string(out), Equals, "ssh -o StrictHostKeyChecking no -q -e someApp 4 /var/lib/tsuru/hooks/dependenciesssh -o StrictHostKeyChecking no -q -e someApp 4 /var/lib/tsuru/hooks/restart")
}

func (s *S) TestLogShouldStoreLog(c *C) {
	a := App{
		Name:    "newApp",
		ec2Auth: &fakeAuthorizer{},
	}
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
	app.setTeams([]auth.Team{s.team, auth.Team{Name: "zzz"}, auth.Team{Name: "aaa"}})
	c.Assert(app.Teams, DeepEquals, []string{"aaa", s.team.Name, "zzz"})
}

func (s *S) TestGetUnits(c *C) {
	app := App{Units: []Unit{Unit{Ip: "1.1.1.1"}}}
	expected := []bind.Unit{bind.Unit(&Unit{Ip: "1.1.1.1", app: &app})}
	c.Assert(app.GetUnits(), DeepEquals, expected)
}

func (s *S) TestDeployShouldCallJujuDeployCommandWithRightEnvironmentInMultiTenantMode(c *C) {
	a := App{
		Name:      "smashed_pumpkin",
		Framework: "golang",
		JujuEnv:   "smashed_pumpkin",
	}
	err := db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	err = deploy(&a)
	c.Assert(err, IsNil)
	logged := strings.Replace(w.String(), "\n", " ", -1)
	expected := ".*deploying golang with name smashed_pumpkin on environment smashed_pumpkin.*"
	c.Assert(logged, Matches, expected)
	expected = ".*deploy -e smashed_pumpkin --repository=/home/charms local:golang smashed_pumpkin.*"
	c.Assert(logged, Matches, expected)
}

func (s *S) TestDeployShouldCallJujuDeployCommandWithRightEnvironmentInSingleTenantMode(c *C) {
	a := App{
		Name:      "smashed_pumpkin",
		Framework: "golang",
		// set to tsuru.conf default (caller's responsibility)
		JujuEnv: "xpto",
	}
	err := db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	err = deploy(&a)
	c.Assert(err, IsNil)
	logged := strings.Replace(w.String(), "\n", " ", -1)
	expected := ".*deploy -e xpto --repository=/home/charms local:golang smashed_pumpkin.*"
	c.Assert(logged, Matches, expected)
}

func (s *S) TestDeployShouldReturnErrorIfAppHasNoJujuEnv(c *C) {
	a := App{
		Name:      "smashed_pumpkin",
		Framework: "golang",
	}
	err := db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	err = deploy(&a)
	expected := "^" + jujuEnvEmptyError.Error() + "$"
	c.Assert(err, ErrorMatches, expected)
}

func (s *S) TestAuthorizeShouldCallEc2Authorizer(c *C) {
	fakeAuth := &fakeAuthorizer{}
	a := App{
		Name:      "smashed_pumpkin",
		Framework: "golang",
		EC2Creds:  map[string]string{"access": "access", "secret": "secret"},
		ec2Auth:   fakeAuth,
	}
	err := db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = authorize(&a)
	c.Assert(err, IsNil)
	action := "authorize " + a.Name
	c.Assert(fakeAuth.hasAction(action), Equals, true)
	action = "setCreds " + a.EC2Creds["access"] + " " + a.EC2Creds["secret"]
	c.Assert(fakeAuth.hasAction(action), Equals, true)
}

func (s *S) TestAuthorizeShouldRepassErrorWhenEc2AuthorizeFails(c *C) {
	fakeAuth := &fakeFailureAuthorizer{}
	a := App{
		Name:      "smashed_pumpkin",
		Framework: "golang",
		EC2Creds:  map[string]string{"access": "access", "secret": "secret"},
		ec2Auth:   fakeAuth,
	}
	err := db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = authorize(&a)
	c.Check(err, NotNil)
	expected := "^Failed to create the app, it was not possible to authorize the access to the app: authorize error$"
	c.Assert(err, ErrorMatches, expected)
}

func (s *S) TestNewEnvironShouldCreateNewJujuEnv(c *C) {
	rfs := s.setupJujuEnviron(c)
	fsystem = rfs
	defer func() {
		fsystem = s.rfs
	}()
	fakeAuth := &fakeAuthorizer{}
	a := App{
		Name:      "myApp",
		Framework: "golang",
		EC2Creds:  map[string]string{"access": "access", "secret": "secret"},
		ec2Auth:   fakeAuth,
		JujuEnv:   "myApp",
	}
	err := db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = newEnviron(&a)
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("openfile "+environConfPath+" with mode 0600"), Equals, true)
}

func (s *S) TestNewEnvironShouldAuthorizeAppGroup(c *C) {
	fakeAuth := &fakeAuthorizer{}
	a := App{
		Name:      "myApp",
		Framework: "golang",
		EC2Creds:  map[string]string{"access": "access", "secret": "secret"},
		ec2Auth:   fakeAuth,
		JujuEnv:   "myApp",
	}
	err := db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = newEnviron(&a)
	c.Assert(err, IsNil)
	action := "authorize " + a.Name
	c.Assert(fakeAuth.hasAction(action), Equals, true)
}

func (s *S) TestCreateAppShouldCreateNewJujuEnvironment(c *C) {
	app := App{
		Name:      "myApp",
		Framework: "golang",
		Teams:     []string{s.team.Name},
		ec2Auth:   &fakeAuthorizer{},
	}
	err := createApp(&app)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	c.Assert(s.rfs.HasAction("openfile "+environConfPath+" with mode 0600"), Equals, true)
}

func (s *S) TestAppDestroyShouldUpdateJujuEnvironment(c *C) {
	app := App{
		Name:      "myApp",
		Framework: "golang",
		Teams:     []string{s.team.Name},
		ec2Auth:   &fakeAuthorizer{},
	}
	err := createApp(&app)
	c.Assert(err, IsNil)
	err = app.destroy()
	c.Assert(err, IsNil)
	file, err := s.rfs.Open(environConfPath)
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	result := map[string]map[string]jujuEnv{}
	goyaml.Unmarshal(content, &result)
	_, ok := result["environments"]["myApp"]
	c.Assert(ok, Equals, false)
}

func (s *S) TestCreateAppShouldSetAppEnvironToDefaultFromConfWhenMultiTenantIsDisabled(c *C) {
	defaultEnv, err := config.GetString("juju:default-env")
	c.Assert(err, IsNil)
	config.Set("multi-tenant", false)
	defer config.Set("multi-tenant", true)
	a := App{
		Name:      "ironic",
		Framework: "ruby",
		Teams:     []string{s.team.Name},
		ec2Auth:   &fakeAuthorizer{},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	c.Assert(a.JujuEnv, Equals, defaultEnv)
}

func (s *S) TestAuthorizerReturnEc2AuthWhenItsNotNil(c *C) {
	auth := &fakeAuthorizer{}
	app := App{Name: "xikin", ec2Auth: auth}
	got := app.authorizer()
	c.Assert(got, DeepEquals, auth)
}

func (s *S) TestAuthorizerInstantiateEc2AuhtorizerWhenEc2AuthIsNul(c *C) {
	app := App{Name: "chico"}
	got := app.authorizer()
	c.Assert(got, FitsTypeOf, &ec2Authorizer{})
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

type fakeAuthorizer struct {
	actions []string
}

func (a *fakeAuthorizer) authorize(app *App) error {
	a.actions = append(a.actions, "authorize "+app.Name)
	return nil
}

func (a *fakeAuthorizer) setCreds(accessKey string, secretKey string) {
	a.actions = append(a.actions, "setCreds "+accessKey+" "+secretKey)
}

func (a *fakeAuthorizer) hasAction(action string) bool {
	for _, v := range a.actions {
		if v == action {
			return true
		}
	}
	return false
}

type fakeFailureAuthorizer struct {
	actions []string
}

func (a *fakeFailureAuthorizer) authorize(app *App) error {
	a.actions = append(a.actions, "authorize "+app.Name)
	return errors.New("authorize error")
}

func (a *fakeFailureAuthorizer) setCreds(accessKey string, secretKey string) {
	a.actions = append(a.actions, "setCreds "+accessKey+" "+secretKey)
}

func (a *fakeFailureAuthorizer) hasAction(action string) bool {
	for _, v := range a.actions {
		if v == action {
			return true
		}
	}
	return false
}
