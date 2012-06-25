package service

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (s *ServiceSuite) createServiceApp() {
	s.serviceType = &ServiceType{Name: "mysql", Charm: "mysql"}
	s.serviceType.Create()
	s.service = &Service{Name: "MySQL", ServiceTypeId: s.serviceType.Id}
	s.service.Create()
	s.app = &app.App{Name: "serviceApp", Framework: "Django"}
	s.app.Create()

	s.serviceApp = &ServiceApp{
		ServiceName: s.service.Name,
		AppName:     s.app.Name,
	}
	s.serviceApp.Create()
}

func (s *ServiceSuite) TestCreateServiceApp(c *C) {
	s.createServiceApp()
	defer s.app.Destroy()
	var result ServiceApp
	query := bson.M{
		"service_name": s.service.Name,
		"app_name":     s.app.Name,
	}
	err := db.Session.ServiceApps().Find(query).One(&result)
	c.Check(err, IsNil)
	c.Assert(s.serviceApp.Id, Not(Equals), "")
	c.Assert(result.ServiceName, Equals, s.service.Name)
	c.Assert(result.AppName, Equals, s.app.Name)
}

func (s *ServiceSuite) TestDeleteServiceApp(c *C) {
	s.createServiceApp()
	defer s.app.Destroy()
	s.serviceApp.Delete()
	query := bson.M{
		"service_name": s.service.Name,
		"app_name":     s.app.Name,
	}
	qtd, err := db.Session.ServiceApps().Find(query).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *ServiceSuite) TestRetrieveAssociatedService(c *C) {
	st := ServiceType{Name: "mysql", Charm: "mysql"}
	st.Create()
	a := app.App{Name: "MyApp", Framework: "Django"}
	a.Create()
	defer a.Destroy()
	service := Service{Name: "my_service", ServiceTypeId: st.Id}
	service.Create()
	serviceApp := &ServiceApp{
		ServiceName: service.Name,
		AppName:     a.Name,
	}
	serviceApp.Create()
	retrievedService := serviceApp.Service()
	c.Assert(service.Name, Equals, retrievedService.Name)
	c.Assert(service.ServiceTypeId, Equals, retrievedService.ServiceTypeId)
}

func (s *ServiceSuite) TestRetrieveAssociatedApp(c *C) {
	a := app.App{Name: "my_app", Framework: "django"}
	a.Create()
	defer a.Destroy()

	st := ServiceType{Name: "mysql", Charm: "mysql"}
	st.Create()

	s.serviceApp = &ServiceApp{
		ServiceName: st.Name,
		AppName:     a.Name,
	}
	s.serviceApp.Create()
	retrievedApp := s.serviceApp.App()
	c.Assert(a.Name, Equals, retrievedApp.Name)
	c.Assert(a.Framework, Equals, retrievedApp.Framework)
}
