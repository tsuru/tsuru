package app

import (
	"bytes"
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo/bson"
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

func (s *S) TestAll(c *C) {
	expected := make([]App, 0)
	app1 := App{Env: make(map[string]string), Name: "app1", Teams: []auth.Team{}, Logs: []Log{}}
	app1.Create()
	expected = append(expected, app1)
	app2 := App{Env: make(map[string]string), Name: "app2", Teams: []auth.Team{}, Logs: []Log{}}
	app2.Create()
	expected = append(expected, app2)
	app3 := App{Env: make(map[string]string), Name: "app3", Teams: []auth.Team{}, Logs: []Log{}}
	app3.Create()
	expected = append(expected, app3)

	appList, err := AllApps()
	c.Assert(err, IsNil)
	for i := 0; i <= 2; i++ {
		c.Assert(expected[i].Name, DeepEquals, appList[i].Name)
		c.Assert(expected[i].State, DeepEquals, appList[i].State)
		expected[i].Destroy()
	}
}

func (s *S) TestGet(c *C) {
	newApp := App{Env: map[string]string{}, Name: "myApp", Framework: "django", Teams: []auth.Team{}, Logs: []Log{}}
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
	a := App{
		Name:      "duvido",
		Framework: "django",
	}
	err := a.Create()
	c.Assert(err, IsNil)
	err = a.Destroy()
	c.Assert(err, IsNil)
	qtd, err := db.Session.Apps().Find(bson.M{"name": a.Name}).Count()
	c.Assert(qtd, Equals, 0)
}

func (s *S) TestCreate(c *C) {
	a := App{}
	a.Name = "appName"
	a.Framework = "django"
	err := a.Create()
	c.Assert(err, IsNil)
	c.Assert(a.State, Equals, "Pending")

	var retrievedApp App
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&retrievedApp)
	c.Assert(err, IsNil)
	c.Assert(retrievedApp.Name, Equals, a.Name)
	c.Assert(retrievedApp.Framework, Equals, a.Framework)
	c.Assert(retrievedApp.State, Equals, a.State)
	a.Destroy()
}

func (s *S) TestCantCreateTwoAppsWithTheSameName(c *C) {
	a := App{Name: "appName", Framework: "django"}
	err := a.Create()
	c.Assert(err, IsNil)

	err = a.Create()
	c.Assert(err, NotNil)

	a.Destroy()
}

func (s *S) TestGrantAccess(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []auth.Team{}}
	err := a.GrantAccess(&s.team)
	c.Assert(err, IsNil)
	c.Assert(s.team, HasAccessTo, a)
}

func (s *S) TestGrantAccessFailsIfTheTeamAlreadyHasAccessToTheApp(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []auth.Team{s.team}}
	err := a.GrantAccess(&s.team)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This team has already access to this app$")
}

func (s *S) TestRevokeAccess(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []auth.Team{s.team}}
	err := a.RevokeAccess(&s.team)
	c.Assert(err, IsNil)
	c.Assert(s.team, Not(HasAccessTo), a)
}

func (s *S) TestRevokeAccessFailsIfTheTeamsDoesNotHaveAccessToTheApp(c *C) {
	a := App{Name: "appName", Framework: "django", Teams: []auth.Team{}}
	err := a.RevokeAccess(&s.team)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This team does not have access to this app$")
}

func (s *S) TestCheckUserAccess(c *C) {
	u := &auth.User{Email: "boy@thewho.com", Password: "123"}
	u2 := &auth.User{Email: "boy2@thewho.com", Password: "123"}
	t := auth.Team{Name: "hello", Users: []*auth.User{u}}
	a := App{Name: "appName", Framework: "django", Teams: []auth.Team{t}}
	c.Assert(a.CheckUserAccess(u), Equals, true)
	c.Assert(a.CheckUserAccess(u2), Equals, false)
}

func (s *S) TestCheckUserAccessWithMultipleUsersOnMultipleGroupsOnApp(c *C) {
	one := &auth.User{Email: "imone@thewho.com", Password: "123"}
	punk := &auth.User{Email: "punk@thewho.com", Password: "123"}
	cut := &auth.User{Email: "cutmyhair@thewho.com", Password: "123"}
	who := auth.Team{Name: "TheWho", Users: []*auth.User{one, punk, cut}}
	what := auth.Team{Name: "TheWhat", Users: []*auth.User{one, punk}}
	where := auth.Team{Name: "TheWhere", Users: []*auth.User{one}}
	a := App{Name: "appppppp", Teams: []auth.Team{who, what, where}}
	c.Assert(a.CheckUserAccess(cut), Equals, true)
	c.Assert(a.CheckUserAccess(punk), Equals, true)
	c.Assert(a.CheckUserAccess(one), Equals, true)
}

func (s *S) TestSetEnvCreatesTheMapIfItIsNil(c *C) {
	a := App{Name: "how-many-more-times"}
	c.Assert(a.Env, IsNil)
	a.SetEnv("PATH", "/")
	c.Assert(a.Env, NotNil)
}

func (s *S) TestSetEnvironmentVariableToApp(c *C) {
	a := App{Name: "appName", Framework: "django"}
	a.SetEnv("PATH", "/")
	c.Assert(a.Env["PATH"], Equals, "/")
}

func (s *S) TestGetEnvironmentVariableFromApp(c *C) {
	a := App{Name: "whole-lotta-love"}
	a.SetEnv("PATH", "/")
	v, err := a.GetEnv("PATH")
	c.Assert(err, IsNil)
	c.Assert(v, Equals, "/")
}

func (s *S) TestGetEnvReturnsErrorIfTheVariableIsNotDeclared(c *C) {
	a := App{Name: "what-is-and-what-should-never"}
	a.Env = make(map[string]string)
	_, err := a.GetEnv("PATH")
	c.Assert(err, NotNil)
}

func (s *S) TestGetEnvReturnsErrorIfTheEnvironmentMapIsNil(c *C) {
	a := App{Name: "what-is-and-what-should-never"}
	_, err := a.GetEnv("PATH")
	c.Assert(err, NotNil)
}

func (s *S) TestUnit(c *C) {
	a := App{Name: "appName", Framework: "django", Machine: 8}
	u := a.unit()
	c.Assert(u, DeepEquals, unit.Unit{Name: a.Name, Type: a.Framework, Machine: a.Machine})
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
	a := App{Name: "something", Framework: "django", Machine: 2}
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
	a := App{Name: "something", Framework: "django", Machine: 2}
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
	a := App{Name: "something", Framework: "django", Machine: 2}
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
	a := App{Name: "something", Framework: "django", Machine: 2}
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
	a := App{Name: "something", Framework: "django", Machine: 2}
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
	a := App{Name: "something", Framework: "django", Machine: 2}
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
	a := App{Name: "something", Framework: "django", Machine: 2}
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
	a := App{Name: "something", Framework: "django", Machine: 2}
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
	a := App{Name: "something", Framework: "django", Machine: 2}
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
	a := App{Name: "something", Framework: "django", Machine: 2}
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	conf, err := a.conf()
	commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	b := a.hasRestartHooks(conf)
	c.Assert(b, Equals, true)
}

func (s *S) TestUpdateHooks(c *C) {
	a := &App{Name: "someApp", Framework: "django", Teams: []auth.Team{s.team}}
	err := a.Create()
	c.Assert(err, IsNil)
	err = a.updateHooks()
	c.Assert(err, IsNil)
}

func (s *S) TestLogShouldStoreLog(c *C) {
	a := App{Name: "newApp"}
	err := a.Create()
	c.Assert(err, IsNil)
	err = a.Log("creating app")
	c.Assert(err, IsNil)
	instance := App{}
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	logLen := len(instance.Logs)
	c.Assert(instance.Logs[logLen-1].Message, Equals, "creating app")
}
