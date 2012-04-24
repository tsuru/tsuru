package service_test

import (
	. "github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/api/service"
	. "github.com/timeredbull/tsuru/database"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo/bson"
)

func (s *ServiceSuite) createServiceApp() {
	s.serviceType = &ServiceType{Name: "mysql", Charm: "mysql"}
	s.serviceType.Create()
	s.service = &Service{Name: "MySQL", ServiceTypeId: s.serviceType.Id}
	s.service.Create()
	s.app = &App{Name: "someApp", Framework: "Django"}
	s.app.Create()

	s.serviceApp = &ServiceApp{
		ServiceId: s.service.Id,
		AppId:     s.app.Id,
	}
	s.serviceApp.Create()
}

func (s *ServiceSuite) TestCreateServiceApp(c *C) {
	s.createServiceApp()
	var result ServiceApp

	collection := Db.C("service_apps")
	query := bson.M{}
	query["service_id"] = s.service.Id
	query["app_id"] = s.app.Id
	err := collection.Find(query).One(&result)
	c.Check(err, IsNil)
	c.Assert(s.serviceApp.Id, Not(Equals), "")
	c.Assert(result.ServiceId, Equals, s.service.Id)
	c.Assert(result.AppId, Equals, s.app.Id)
}

func (s *ServiceSuite) TestDeleteServiceApp(c *C) {
	s.createServiceApp()
	s.serviceApp.Delete()

	collection := Db.C("service_apps")
	query := bson.M{}
	query["service_id"] = s.service.Id
	query["app_id"] = s.app.Id

	qtd, err := collection.Find(query).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *ServiceSuite) TestRetrieveAssociatedService(c *C) {
	st := ServiceType{Name: "mysql", Charm: "mysql"}
	st.Create()
	a := App{Name: "MyApp", Framework: "Django"}
	a.Create()
	service := Service{Name: "my_service", ServiceTypeId: st.Id}
	service.Create()

	serviceApp := &ServiceApp{
		ServiceId: service.Id,
		AppId:     a.Id,
	}
	serviceApp.Create()

	retrievedService := serviceApp.Service()

	c.Assert(service.Name, Equals, retrievedService.Name)
	c.Assert(service.Id, Equals, retrievedService.Id)
	c.Assert(service.ServiceTypeId, Equals, retrievedService.ServiceTypeId)
}

func (s *ServiceSuite) TestRetrieveAssociatedApp(c *C) {
	app := App{Name: "my_app", Framework: "django"}
	app.Create()

	st := ServiceType{Name: "mysql", Charm: "mysql"}
	st.Create()

	s.serviceApp = &ServiceApp{
		ServiceId: st.Id,
		AppId:     app.Id,
	}
	s.serviceApp.Create()

	retrievedApp := s.serviceApp.App()

	c.Assert(app.Name, Equals, retrievedApp.Name)
	c.Assert(app.Framework, Equals, retrievedApp.Framework)
}
