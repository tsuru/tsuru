package app

import (
	"bytes"
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/bind"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	stdlog "log"
	"strings"
)

type hasAccessToChecker struct{}

func (c *hasAccessToChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "HasAccessTo", Params: []string{"team", "app"}}
}

func (c *hasAccessToChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you must provide two parameters"
	}
	team, ok := params[0].(auth.Team)
	if !ok {
		return false, "first parameter should be a team instance"
	}
	app, ok := params[1].(App)
	if !ok {
		return false, "second parameter should be an app instance"
	}
	return app.hasTeam(&team), ""
}

var HasAccessTo Checker = &hasAccessToChecker{}

func (s *S) TestGet(c *C) {
	newApp := App{Env: map[string]EnvVar{}, Name: "myApp", Framework: "django", Teams: []string{}, Logs: []Log{}}
	err := newApp.Create()
	c.Assert(err, IsNil)

	myApp := App{Name: "myApp"}
	err = myApp.Get()
	c.Assert(err, IsNil)
	c.Assert(myApp.Name, Equals, newApp.Name)
	c.Assert(myApp.State, Equals, newApp.State)

	err = myApp.Destroy()
	c.Assert(err, IsNil)
}

func (s *S) TestDestroy(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	u := unit.Unit{Name: "duvido", Machine: 3}
	a := App{Name: "duvido", Framework: "django", Units: []unit.Unit{u}}
	err = a.Create()
	c.Assert(err, IsNil)
	err = a.Destroy()
	c.Assert(err, IsNil)
	logStr := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(logStr, Matches, ".*destroy-service duvido.*")
	c.Assert(logStr, Matches, ".*terminate-machine 3.*")
	qtd, err := db.Session.Apps().Find(bson.M{"name": a.Name}).Count()
	c.Assert(qtd, Equals, 0)
}

func (s *S) TestCreate(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	a := App{Name: "appName", Framework: "django"}
	err = a.Create()
	c.Assert(err, IsNil)
	c.Assert(a.State, Equals, "pending")
	defer a.Destroy()

	var retrievedApp App
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&retrievedApp)
	c.Assert(err, IsNil)
	c.Assert(retrievedApp.Name, Equals, a.Name)
	c.Assert(retrievedApp.Framework, Equals, a.Framework)
	c.Assert(retrievedApp.State, Equals, a.State)
	c.Assert(retrievedApp.JujuEnv, Equals, "delta")
	str := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(str, Matches, ".*deploy --repository=/home/charms local:django appName.*")
}

func (s *S) TestCantCreateTwoAppsWithTheSameName(c *C) {
	a := App{Name: "appName", Framework: "django"}
	err := a.Create()
	c.Assert(err, IsNil)

	err = a.Create()
	c.Assert(err, NotNil)

	a.Destroy()
}

func (s *S) TestAppendOrUpdate(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{}}
	a.Create()
	defer a.Destroy()
	u := unit.Unit{Name: "someapp", Ip: "", Machine: 3, InstanceId: "i-00000zz8"}
	a.AddOrUpdateUnit(&u)
	c.Assert(len(a.Units), Equals, 1)
	u = unit.Unit{Name: "someapp", Ip: "192.168.0.12", Machine: 3, InstanceId: "i-00000zz8"}
	a.AddOrUpdateUnit(&u)
	c.Assert(len(a.Units), Equals, 1)
	c.Assert(a.Units[0].Ip, Equals, "192.168.0.12")
}

func (s *S) TestGrantAccess(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{}}
	err := a.GrantAccess(&s.team)
	c.Assert(err, IsNil)
	c.Assert(s.team, HasAccessTo, a)
}

func (s *S) TestGrantAccessFailsIfTheTeamAlreadyHasAccessToTheApp(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{s.team.Name}}
	err := a.GrantAccess(&s.team)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This team has already access to this app$")
}

func (s *S) TestRevokeAccess(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{s.team.Name}}
	err := a.RevokeAccess(&s.team)
	c.Assert(err, IsNil)
	c.Assert(s.team, Not(HasAccessTo), a)
}

func (s *S) TestRevokeAccessFailsIfTheTeamsDoesNotHaveAccessToTheApp(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []string{}}
	err := a.RevokeAccess(&s.team)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This team does not have access to this app$")
}

func (s *S) TestSetEnvCreatesTheMapIfItIsNil(c *C) {
	a := App{Name: "how-many-more-times"}
	c.Assert(a.Env, IsNil)
	env := EnvVar{Name: "PATH", Value: "/"}
	a.SetEnv(env)
	c.Assert(a.Env, NotNil)
}

func (s *S) TestSetEnvironmentVariableToApp(c *C) {
	a := App{Name: "appName", Framework: "django"}
	a.SetEnv(EnvVar{Name: "PATH", Value: "/", Public: true})
	env := a.Env["PATH"]
	c.Assert(env.Name, Equals, "PATH")
	c.Assert(env.Value, Equals, "/")
	c.Assert(env.Public, Equals, true)
}

func (s *S) TestGetEnvironmentVariableFromApp(c *C) {
	a := App{Name: "whole-lotta-love"}
	a.SetEnv(EnvVar{Name: "PATH", Value: "/"})
	v, err := a.GetEnv("PATH")
	c.Assert(err, IsNil)
	c.Assert(v.Value, Equals, "/")
}

func (s *S) TestGetEnvReturnsErrorIfTheVariableIsNotDeclared(c *C) {
	a := App{Name: "what-is-and-what-should-never"}
	a.Env = make(map[string]EnvVar)
	_, err := a.GetEnv("PATH")
	c.Assert(err, NotNil)
}

func (s *S) TestGetEnvReturnsErrorIfTheEnvironmentMapIsNil(c *C) {
	a := App{Name: "what-is-and-what-should-never"}
	_, err := a.GetEnv("PATH")
	c.Assert(err, NotNil)
}

func (s *S) TestInstanceEnvironmentReturnEnvironmentVariablesForTheServer(c *C) {
	envs := map[string]EnvVar{
		"DATABASE_HOST": EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: false, InstanceName: "mysql"},
		"DATABASE_USER": EnvVar{Name: "DATABASE_USER", Value: "root", Public: true, InstanceName: "mysql"},
		"HOST":          EnvVar{Name: "HOST", Value: "10.0.2.1", Public: false, InstanceName: "redis"},
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
	u := unit.Unit{Name: "someapp/0", Type: "django", Machine: 10}
	a := App{Name: "appName", Framework: "django", Units: []unit.Unit{u}}
	u2 := a.unit()
	c.Assert(u2, DeepEquals, u)
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
	a := App{Name: "something", Framework: "django"}
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
	a := App{Name: "something", Framework: "django"}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	dir, err = commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	err = a.preRestart(conf)
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	c.Assert(st[len(st)-2], Matches, ".*/bin/bash /home/application/current/pre.sh$")
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
	err = a.preRestart(conf)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	c.Assert(st[len(st)-2], Matches, ".*app.conf file does not exists or is in the right place. Skipping...")
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
	a := App{Name: "something", Framework: "django"}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	err = a.preRestart(conf)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	c.Assert(st[len(st)-2], Matches, ".*pre-restart hook section in app conf does not exists... Skipping...")
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
	a := App{Name: "something", Framework: "django"}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	dir, err = commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	err = a.posRestart(conf)
	c.Assert(err, IsNil)
	commandmocker.Remove(dir)
	st := strings.Split(w.String(), "\n")
	c.Assert(st[len(st)-2], Matches, ".*/bin/bash /home/application/current/pos.sh$")
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
	err = a.posRestart(conf)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	c.Assert(st[len(st)-2], Matches, ".*app.conf file does not exists or is in the right place. Skipping...")
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
	a := App{Name: "something", Framework: "django"}
	conf, err := a.conf()
	c.Assert(err, IsNil)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	err = a.posRestart(conf)
	c.Assert(err, IsNil)
	st := strings.Split(w.String(), "\n")
	c.Assert(st[len(st)-2], Matches, ".*pos-restart hook section in app conf does not exists... Skipping...")
}

func (s *S) TestHasRestartHooksWithNoHooks(c *C) {
	output := `
something that must be discarded
========
nothing here
`
	a := App{Name: "something", Framework: "django"}
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
	a := App{Name: "something", Framework: "django"}
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	conf, err := a.conf()
	commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	b := a.hasRestartHooks(conf)
	c.Assert(b, Equals, true)
}

func (s *S) TestUpdateHooks(c *C) {
	a := &App{Name: "someApp", Framework: "django", Teams: []string{s.team.Name}}
	err := a.Create()
	c.Assert(err, IsNil)
	err = a.updateHooks()
	c.Assert(err, IsNil)
}

func (s *S) TestLogShouldStoreLog(c *C) {
	a := App{Name: "newApp"}
	err := a.Create()
	c.Assert(err, IsNil)
	err = a.Log("last log msg")
	c.Assert(err, IsNil)
	instance := App{}
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	logLen := len(instance.Logs)
	c.Assert(instance.Logs[logLen-1].Message, Equals, "last log msg")
}

func (s *S) TestAppShouldStoreUnits(c *C) {
	u := unit.Unit{Name: "someapp/0", Type: "django"}
	units := []unit.Unit{u}
	var instance App
	a := App{Name: "someApp", Units: units}
	err := a.Create()
	c.Assert(err, IsNil)
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	c.Assert(err, IsNil)
	c.Assert(len(instance.Units), Equals, 1)
}

func (s *S) TestEnvVarStringPrintPublicValue(c *C) {
	env := EnvVar{Name: "PATH", Value: "/", Public: true}
	c.Assert(env.String(), Equals, "PATH=/")
}

func (s *S) TestEnvVarStringMaskPrivateValue(c *C) {
	env := EnvVar{Name: "PATH", Value: "/", Public: false}
	c.Assert(env.String(), Equals, "PATH=*** (private variable)")
}

func (s *S) TestGetTeams(c *C) {
	app := App{Name: "app", Teams: []string{s.team.Name}}
	teams := app.GetTeams()
	c.Assert(teams, HasLen, 1)
	c.Assert(teams[0].Name, Equals, s.team.Name)
}

func (s *S) TestSetTeams(c *C) {
	app := App{Name: "app"}
	app.setTeams([]auth.Team{s.team})
	c.Assert(app.Teams, DeepEquals, []string{s.team.Name})
}

func (s *S) TestGetUnits(c *C) {
	app := App{Units: []unit.Unit{unit.Unit{Ip: "1.1.1.1"}}}
	c.Assert(app.GetUnits(), DeepEquals, []unit.Unit{unit.Unit{Ip: "1.1.1.1"}})
}
