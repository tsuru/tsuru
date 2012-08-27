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
	s.ts.Close()
	keystoneTs := s.mockServer("", "", "", "")
	authUrl = keystoneTs.URL
	defer func() {
		authUrl = ""
		keystoneTs.Close()
	}()
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
		ec2Auth:   &fakeAuthorizer{},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.KeystoneEnv = keystoneEnv{
		TenantId:  "e60d1f0a-ee74-411c-b879-46aee9502bf9",
		UserId:    "1b4d1195-7890-4274-831f-ddf8141edecc",
		AccessKey: "91232f6796b54ca2a2b87ef50548b123",
	}
	a.Units = []Unit{Unit{Ip: "10.10.10.10"}}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	c.Assert(err, IsNil)
	err = a.destroy()
	c.Assert(err, IsNil)
	n, _ := db.Session.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{a.Name}}}).Count()
	c.Assert(n, Equals, 0)
}
