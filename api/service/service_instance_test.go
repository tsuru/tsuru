package service

import (
	"github.com/timeredbull/tsuru/api/bind"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (s *S) createServiceInstance() {
	s.service = &Service{Name: "MySQL"}
	s.service.Create()
	s.serviceInstance = &ServiceInstance{
		Name: s.service.Name,
	}
	s.serviceInstance.Create()
}

func (s *S) TestCreateServiceInstance(c *C) {
	s.createServiceInstance()
	defer db.Session.Services().Remove(bson.M{"_id": s.service.Name})
	var result ServiceInstance
	query := bson.M{
		"_id": s.service.Name,
	}
	err := db.Session.ServiceInstances().Find(query).One(&result)
	c.Check(err, IsNil)
	c.Assert(result.Name, Equals, s.service.Name)
}

func (s *S) TestDeleteServiceInstance(c *C) {
	s.createServiceInstance()
	defer db.Session.Services().Remove(bson.M{"_id": s.service.Name})
	s.serviceInstance.Delete()
	query := bson.M{
		"_id": s.service.Name,
	}
	qtd, err := db.Session.ServiceInstances().Find(query).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *S) TestRetrieveAssociatedService(c *C) {
	service := Service{Name: "my_service"}
	service.Create()
	serviceInstance := &ServiceInstance{
		Name:        service.Name,
		ServiceName: service.Name,
	}
	serviceInstance.Create()
	rService := serviceInstance.Service()
	c.Assert(service.Name, Equals, rService.Name)
}

func (s *S) TestAddApp(c *C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{},
	}
	err := instance.AddApp("app1")
	c.Assert(err, IsNil)
	c.Assert(instance.Apps, DeepEquals, []string{"app1"})
}

func (s *S) TestAddAppReturnErrorIfTheAppIsAlreadyPresent(c *C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1"},
	}
	err := instance.AddApp("app1")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This instance already has this app.$")
}

func (s *S) TestFindApp(c *C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1", "app2"},
	}
	c.Assert(instance.FindApp("app1"), Equals, 0)
	c.Assert(instance.FindApp("app2"), Equals, 1)
	c.Assert(instance.FindApp("what"), Equals, -1)
}

func (s *S) TestRemoveApp(c *C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1", "app2", "app3"},
	}
	err := instance.RemoveApp("app2")
	c.Assert(err, IsNil)
	c.Assert(instance.Apps, DeepEquals, []string{"app1", "app3"})
	err = instance.RemoveApp("app3")
	c.Assert(err, IsNil)
	c.Assert(instance.Apps, DeepEquals, []string{"app1"})
}

func (s *S) TestRemoveAppReturnsErrorWhenTheAppIsNotBindedToTheInstance(c *C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1", "app2", "app3"},
	}
	err := instance.RemoveApp("app4")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This app is not binded to this service instance.$")
}

func (s *S) TestServiceInstanceIsAnAppContainer(c *C) {
	var container bind.AppContainer
	c.Assert(&ServiceInstance{}, Implements, &container)
}

func (s *S) TestServiceInstanceIsABinder(c *C) {
	var binder bind.Binder
	c.Assert(&ServiceInstance{}, Implements, &binder)
}
