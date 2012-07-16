package service

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/bind"
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
		State:    "creating",
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
	c.Assert(result.State, Equals, "creating")
}

func (s *S) TestCreateServiceInstanceShouldSetTheStateToCreatingOnlyIfTheStateIsNotDefined(c *C) {
	instance1 := ServiceInstance{Name: "instance1", State: "created"}
	err := instance1.Create()
	c.Assert(err, IsNil)
	defer instance1.Delete()
	instance2 := ServiceInstance{Name: "instance2"}
	err = instance2.Create()
	c.Assert(err, IsNil)
	defer instance1.Delete()
	c.Assert(instance1.State, Equals, "created")
	c.Assert(instance2.State, Equals, "creating")
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
