package service

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (s *S) createServiceInstance() {
	s.service = &Service{Name: "MySQL"}
	s.service.Create()
	s.app = &app.App{Name: "serviceInstance", Framework: "Django"}
	s.app.Create()
	s.serviceInstance = &ServiceInstance{
		Name:     s.service.Name,
		Apps:     []string{s.app.Name},
		Instance: "i-000000a",
		State:    "CREATING",
	}
	s.serviceInstance.Create()
}

func (s *S) TestCreateServiceInstance(c *C) {
	s.createServiceInstance()
	defer s.app.Destroy()
	defer s.service.Delete()
	var result ServiceInstance
	query := bson.M{
		"_id":  s.service.Name,
		"apps": []string{s.app.Name},
	}
	err := db.Session.ServiceInstances().Find(query).One(&result)
	c.Check(err, IsNil)
	c.Assert(result.Name, Equals, s.service.Name)
	c.Assert(result.Apps[0], Equals, s.app.Name)
	c.Assert(result.Instance, Equals, "i-000000a")
	c.Assert(result.State, Equals, "CREATING")
}

func (s *S) TestDeleteServiceInstance(c *C) {
	s.createServiceInstance()
	defer s.app.Destroy()
	defer s.service.Delete()
	s.serviceInstance.Delete()
	query := bson.M{
		"_id":  s.service.Name,
		"apps": []string{s.app.Name},
	}
	qtd, err := db.Session.ServiceInstances().Find(query).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *S) TestRetrieveAssociatedService(c *C) {
	a := app.App{Name: "MyApp", Framework: "Django"}
	a.Create()
	defer a.Destroy()
	service := Service{Name: "my_service"}
	service.Create()
	serviceInstance := &ServiceInstance{
		Name:        service.Name,
		Apps:        []string{a.Name},
		ServiceName: service.Name,
	}
	serviceInstance.Create()
	rService := serviceInstance.Service()
	c.Assert(service.Name, Equals, rService.Name)
}

func (s *S) TestRetrieveAssociatedApp(c *C) {
	a := app.App{Name: "my_app", Framework: "django"}
	a.Create()
	defer a.Destroy()
	s.serviceInstance = &ServiceInstance{
		Name: "my_mysql",
		Apps: []string{a.Name},
	}
	s.serviceInstance.Create()
	rApp := s.serviceInstance.AllApps()[0]
	c.Assert(a.Name, Equals, rApp.Name)
	c.Assert(a.Framework, Equals, rApp.Framework)
}
