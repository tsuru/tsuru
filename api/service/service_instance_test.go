package service

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (s *ServiceSuite) createServiceInstance() {
	s.serviceType = &ServiceType{Name: "mysql", Charm: "mysql"}
	s.serviceType.Create()
	s.service = &Service{Name: "MySQL", ServiceTypeName: s.serviceType.Name}
	s.service.Create()
	s.app = &app.App{Name: "serviceInstance", Framework: "Django"}
	s.app.Create()
	s.serviceInstance = &ServiceInstance{
		Name: s.service.Name,
		Apps: []string{s.app.Name},
	}
	s.serviceInstance.Create()
}

func (s *ServiceSuite) TestCreateServiceInstance(c *C) {
	s.createServiceInstance()
	defer s.app.Destroy()
	defer s.service.Delete()
	defer s.serviceType.Delete()
	var result ServiceInstance
	query := bson.M{
		"_id":  s.service.Name,
		"apps": []string{s.app.Name},
	}
	err := db.Session.ServiceInstances().Find(query).One(&result)
	c.Check(err, IsNil)
	c.Assert(result.Name, Equals, s.service.Name)
	c.Assert(result.Apps[0], Equals, s.app.Name)
}

func (s *ServiceSuite) TestDeleteServiceInstance(c *C) {
	s.createServiceInstance()
	defer s.app.Destroy()
	defer s.service.Delete()
	defer s.serviceType.Delete()
	s.serviceInstance.Delete()
	query := bson.M{
		"_id":  s.service.Name,
		"apps": []string{s.app.Name},
	}
	qtd, err := db.Session.ServiceInstances().Find(query).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *ServiceSuite) TestRetrieveAssociatedService(c *C) {
	st := ServiceType{Name: "mysql", Charm: "mysql"}
	st.Create()
	a := app.App{Name: "MyApp", Framework: "Django"}
	a.Create()
	defer a.Destroy()
	defer st.Delete()
	service := Service{Name: "my_service", ServiceTypeName: st.Name}
	service.Create()
	serviceInstance := &ServiceInstance{
		Name:        service.Name,
		Apps:        []string{a.Name},
		ServiceName: service.Name,
	}
	serviceInstance.Create()
	rService := serviceInstance.Service()
	c.Assert(service.Name, Equals, rService.Name)
	c.Assert(service.ServiceTypeName, Equals, rService.ServiceTypeName)
}

func (s *ServiceSuite) TestRetrieveAssociatedApp(c *C) {
	a := app.App{Name: "my_app", Framework: "django"}
	a.Create()
	defer a.Destroy()
	st := ServiceType{Name: "mysql", Charm: "mysql"}
	st.Create()

	s.serviceInstance = &ServiceInstance{
		Name: st.Name,
		Apps: []string{a.Name},
	}
	s.serviceInstance.Create()
	rApp := s.serviceInstance.AllApps()[0]
	c.Assert(a.Name, Equals, rApp.Name)
	c.Assert(a.Framework, Equals, rApp.Framework)
}
