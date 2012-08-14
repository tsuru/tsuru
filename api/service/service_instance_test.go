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

func (s *S) TestGetServiceInstancesByService(c *C) {
	srvc := Service{Name: "mysql"}
	err := srvc.Create()
	c.Assert(err, IsNil)
	sInstance := ServiceInstance{Name: "t3sql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = sInstance.Create()
	c.Assert(err, IsNil)
	sInstance2 := ServiceInstance{Name: "s9sql", ServiceName: "mysql"}
	err = sInstance2.Create()
	c.Assert(err, IsNil)
	sInstances, err := GetServiceInstancesByService(srvc)
	c.Assert(err, IsNil)
	expected := []ServiceInstance{ServiceInstance{Name: "t3sql", ServiceName: "mysql"}, sInstance2}
	c.Assert(sInstances, DeepEquals, expected)
}

func (s *S) TestGetServiceInstancesByServiceWithoutAnyExistingServiceInstances(c *C) {
	srvc := Service{Name: "mysql"}
	err := srvc.Create()
	c.Assert(err, IsNil)
	sInstances, err := GetServiceInstancesByService(srvc)
	c.Assert(err, IsNil)
	c.Assert(sInstances, DeepEquals, []ServiceInstance(nil))
}

func (s *S) TestGenericServiceInstancesFilter(c *C) {
	srvc := Service{Name: "mysql"}
	teams := []string{s.team.Name}
	q, f := genericServiceInstancesFilter(srvc, teams)
	c.Assert(q, DeepEquals, bson.M{"service_name": srvc.Name, "teams": bson.M{"$in": teams}})
	c.Assert(f, DeepEquals, bson.M{"name": 1, "service_name": 1, "apps": 1})
}

func (s *S) TestGenericServiceInstancesFilterWithServiceSlice(c *C) {
	services := []Service{
		Service{Name: "mysql"},
		Service{Name: "mongodb"},
	}
	names := []string{"mysql", "mongodb"}
	teams := []string{s.team.Name}
	q, f := genericServiceInstancesFilter(services, teams)
	c.Assert(q, DeepEquals, bson.M{"service_name": bson.M{"$in": names}, "teams": bson.M{"$in": teams}})
	c.Assert(f, DeepEquals, bson.M{"name": 1, "service_name": 1, "apps": 1})
}

func (s *S) TestGetServiceInstancesByServiceAndTeams(c *C) {
	srvc := Service{Name: "mysql", Teams: []string{s.team.Name}}
	srvc.Create()
	defer srvc.Delete()
	sInstance := ServiceInstance{
		Name:        "j4sql",
		ServiceName: srvc.Name,
		Teams:       []string{s.team.Name},
	}
	sInstance.Create()
	defer sInstance.Delete()
	sInstances, err := GetServiceInstancesByServiceAndTeams(srvc, s.user)
	c.Assert(err, IsNil)
	expected := []ServiceInstance{
		ServiceInstance{
			Name:        sInstance.Name,
			ServiceName: sInstance.ServiceName,
			Teams:       []string(nil),
			Apps:        []string{},
		},
	}
	c.Assert(sInstances, DeepEquals, expected)
}

func (s *S) TestGetServiceInstancesByServiceAndTeamsIgnoresIsRestrictedFlagFromService(c *C) {
	srvc := Service{Name: "mysql", Teams: []string{s.team.Name}, IsRestricted: true}
	srvc.Create()
	defer srvc.Delete()
	sInstance := ServiceInstance{
		Name:        "j4sql",
		ServiceName: srvc.Name,
		Teams:       []string{s.team.Name},
	}
	sInstance.Create()
	defer sInstance.Delete()
	sInstances, err := GetServiceInstancesByServiceAndTeams(srvc, s.user)
	c.Assert(err, IsNil)
	expected := []ServiceInstance{
		ServiceInstance{
			Name:        sInstance.Name,
			ServiceName: sInstance.ServiceName,
			Teams:       []string(nil),
			Apps:        []string{},
		},
	}
	c.Assert(sInstances, DeepEquals, expected)
}

func (s *S) TestGetServiceInstancesByServicesAndTeams(c *C) {
	srvc := Service{Name: "mysql", Teams: []string{s.team.Name}, IsRestricted: true}
	srvc.Create()
	defer srvc.Delete()
	srvc2 := Service{Name: "mongodb", Teams: []string{s.team.Name}, IsRestricted: false}
	srvc2.Create()
	defer srvc2.Delete()
	sInstance := ServiceInstance{
		Name:        "j4sql",
		ServiceName: srvc.Name,
		Teams:       []string{s.team.Name},
	}
	sInstance.Create()
	defer sInstance.Delete()
	sInstance2 := ServiceInstance{
		Name:        "j4nosql",
		ServiceName: srvc2.Name,
		Teams:       []string{s.team.Name},
	}
	sInstance2.Create()
	defer sInstance2.Delete()
	sInstance3 := ServiceInstance{
		Name:        "f9nosql",
		ServiceName: srvc2.Name,
	}
	sInstance3.Create()
	defer sInstance3.Delete()
	expected := []ServiceInstance{
		ServiceInstance{
			Name:        sInstance.Name,
			ServiceName: sInstance.ServiceName,
			Teams:       []string(nil),
			Apps:        []string{},
		},
		ServiceInstance{
			Name:        sInstance2.Name,
			ServiceName: sInstance2.ServiceName,
			Teams:       []string(nil),
			Apps:        []string{},
		},
	}
	sInstances, err := GetServiceInstancesByServicesAndTeams([]Service{srvc, srvc2}, s.user)
	c.Assert(err, IsNil)
	c.Assert(sInstances, DeepEquals, expected)
}
