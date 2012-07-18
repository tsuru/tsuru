package app

import (
	"github.com/timeredbull/tsuru/api/bind"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (s *S) TestAppIsABinderApp(c *C) {
	var app bind.App
	c.Assert(&App{}, Implements, &app)
}

func (s *S) TestDestroyShouldUnbindAppFromInstance(c *C) {
	instance := service.ServiceInstance{Name: "MyInstance", Apps: []string{"myApp"}}
	err := instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": instance.Name})
	a := App{Name: "myApp"}
	err = a.Create()
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = a.Destroy()
	c.Assert(err, IsNil)
	n, _ := db.Session.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{a.Name}}}).Count()
	c.Assert(n, Equals, 0)
}
