package service

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/ec2"
	"io/ioutil"
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
	"testing"
)

type hasAccessToChecker struct{}

func (c *hasAccessToChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "HasAccessTo", Params: []string{"team", "service"}}
}

func (c *hasAccessToChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you must provide two parameters"
	}
	team, ok := params[0].(auth.Team)
	if !ok {
		return false, "first parameter should be a team instance"
	}
	service, ok := params[1].(Service)
	if !ok {
		return false, "second parameter should be service instance"
	}
	return service.hasTeam(&team), ""
}

var HasAccessTo Checker = &hasAccessToChecker{}

func Test(t *testing.T) { TestingT(t) }

type S struct {
	app             *app.App
	service         *Service
	serviceInstance *ServiceInstance
	team            *auth.Team
	user            *auth.User
	tmpdir          string
	ec2Srv          *ec2test.Server
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	s.ec2Srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatal(err)
	}
	s.setupConfig(c)
	s.tmpdir, err = commandmocker.Add("juju", "")
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_service_test")
	c.Assert(err, IsNil)
	s.user = &auth.User{Email: "cidade@raul.com", Password: "123"}
	err = s.user.Create()
	c.Assert(err, IsNil)
	s.team = &auth.Team{Name: "Raul", Users: []auth.User{*s.user}}
	err = db.Session.Teams().Insert(s.team)
	c.Assert(err, IsNil)
	s.reconfEc2Srv(c)
	_, err = ec2.Conn()
	if err != nil {
		c.Fail()
	}
}

func (s *S) TearDownSuite(c *C) {
	defer commandmocker.Remove(s.tmpdir)
	defer db.Session.Close()
	s.ec2Srv.Quit()
	db.Session.Apps().Database.DropDatabase()
}

func (s *S) TearDownTest(c *C) {
	_, err := db.Session.Services().RemoveAll(nil)
	c.Assert(err, IsNil)

	_, err = db.Session.ServiceInstances().RemoveAll(nil)
	c.Assert(err, IsNil)

	var apps []app.App
	err = db.Session.Apps().Find(nil).All(&apps)
	c.Assert(err, IsNil)
	for _, a := range apps {
		err = a.Destroy()
		c.Assert(err, IsNil)
	}
}

func (s *S) setupConfig(c *C) {
	data, err := ioutil.ReadFile("../../etc/tsuru.conf")
	if err != nil {
		c.Fatal(err)
	}
	err = config.ReadConfigBytes(data)
	if err != nil {
		c.Fatal(err)
	}
}

func (s *S) reconfEc2Srv(c *C) {
	config.Set("aws:ec2-endpoint", s.ec2Srv.URL())
	config.Set("aws:secret-key", "blee")
	config.Set("aws:access-key", "blaa")
}
