package service

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
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

type ServiceSuite struct {
	app         *app.App
	service     *Service
	serviceType *ServiceType
	serviceApp  *ServiceApp
	team        *auth.Team
	user        *auth.User
}

var _ = Suite(&ServiceSuite{})

func (s *ServiceSuite) SetUpSuite(c *C) {
	var err error
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_service_test")
	c.Assert(err, IsNil)
	s.user = &auth.User{Email: "cidade@raul.com", Password: "123"}
	err = s.user.Create()
	c.Assert(err, IsNil)
	s.team = &auth.Team{Name: "Raul", Users: []*auth.User{s.user}}
	err = db.Session.Teams().Insert(s.team)
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TearDownSuite(c *C) {
	defer db.Session.Close()
	db.Session.DropDB()
}

func (s *ServiceSuite) TearDownTest(c *C) {
	err := db.Session.Services().RemoveAll(nil)
	c.Assert(err, IsNil)

	err = db.Session.ServiceApps().RemoveAll(nil)
	c.Assert(err, IsNil)

	err = db.Session.ServiceTypes().RemoveAll(nil)
	c.Assert(err, IsNil)

	var apps []app.App
	err = db.Session.Apps().Find(nil).All(&apps)
	c.Assert(err, IsNil)
	for _, a := range apps {
		a.Destroy()
	}
}
