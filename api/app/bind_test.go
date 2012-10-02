package app

import (
	"github.com/timeredbull/tsuru/api/bind"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestAppIsABinderApp(c *C) {
	var app bind.App
	c.Assert(&App{}, Implements, &app)
}

func (s *S) TestDestroyShouldUnbindAppFromInstance(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "my", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": srvc.Name})
	instance := service.ServiceInstance{Name: "MyInstance", Apps: []string{"myApp"}, ServiceName: srvc.Name}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": instance.Name})
	a := App{
		Name:      "myApp",
		Framework: "",
		Teams:     []string{},
		EC2Creds:  map[string]string{"access": "91232f6796b54ca2a2b87ef50548b123"},
		Units: []Unit{
			Unit{Ip: "10.10.10.10"},
		},
		ec2Auth: &fakeAuthorizer{},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = a.destroy()
	c.Assert(err, IsNil)
	n, _ := db.Session.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{a.Name}}}).Count()
	c.Assert(n, Equals, 0)
}
