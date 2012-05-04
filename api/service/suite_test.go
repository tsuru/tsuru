package service

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type ServiceSuite struct {
	app         *app.App
	service     *Service
	serviceType *ServiceType
	serviceApp  *ServiceApp
	session     *mgo.Session
}

var _ = Suite(&ServiceSuite{})

func (s *ServiceSuite) SetUpSuite(c *C) {
	var err error
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_service_test")
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
}
