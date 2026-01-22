// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	provTypes "github.com/tsuru/tsuru/types/provision"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	check "gopkg.in/check.v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type InstanceSuite struct {
	team        *authTypes.Team
	user        *auth.User
	mockService servicemock.MockService
}

var _ = check.Suite(&InstanceSuite{})

func (s *InstanceSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_service_instance_test")

	storagev2.Reset()
}

func (s *InstanceSuite) SetUpTest(c *check.C) {
	routertest.FakeRouter.Reset()
	storagev2.ClearAllCollections(nil)
	s.user = &auth.User{Email: "cidade@raul.com", Password: "123"}
	s.team = &authTypes.Team{Name: "raul"}

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)

	_, err = usersCollection.InsertOne(context.TODO(), s.user)
	c.Assert(err, check.IsNil)

	servicemock.SetMockService(&s.mockService)
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		if name == s.team.Name {
			return s.team, nil
		}
		return nil, authTypes.ErrTeamNotFound
	}
	s.mockService.Team.OnFindByNames = func(names []string) ([]authTypes.Team, error) {
		return []authTypes.Team{*s.team}, nil
	}

	s.mockService.App.OnGetAddresses = func(a *appTypes.App) ([]string, error) {
		return routertest.FakeRouter.Addresses(context.TODO(), a)
	}
	s.mockService.App.OnGetInternalBindableAddresses = func(a *appTypes.App) ([]string, error) {
		return []string{}, nil
	}
	s.mockService.App.OnAddInstance = func(a *appTypes.App, instanceArgs bindTypes.AddInstanceArgs) error {
		a.ServiceEnvs = append(a.ServiceEnvs, instanceArgs.Envs...)
		if instanceArgs.Writer != nil {
			instanceArgs.Writer.Write([]byte("add instance"))
		}

		return nil
	}
	s.mockService.App.OnRemoveInstance = func(a *appTypes.App, instanceArgs bindTypes.RemoveInstanceArgs) error {
		lenBefore := len(a.ServiceEnvs)
		for i := 0; i < len(a.ServiceEnvs); i++ {
			se := a.ServiceEnvs[i]
			if se.ServiceName == instanceArgs.ServiceName && se.InstanceName == instanceArgs.InstanceName {
				a.ServiceEnvs = append(a.ServiceEnvs[:i], a.ServiceEnvs[i+1:]...)
				i--
			}
		}
		if len(a.ServiceEnvs) == lenBefore {
			return errors.New("instance not found")
		}
		if instanceArgs.Writer != nil {
			instanceArgs.Writer.Write([]byte("remove instance"))
		}
		return nil
	}

}

func (s *InstanceSuite) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func (s *InstanceSuite) TestDeleteServiceInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && strings.HasSuffix(r.URL.Path, "/resources/MySQL") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	err := Create(context.TODO(), Service{
		Name:       "mongodb",
		Password:   "password",
		OwnerTeams: []string{"raul"},
		Endpoint: map[string]string{
			"production": ts.URL,
		},
	})
	c.Assert(err, check.IsNil)
	si := &ServiceInstance{Name: "MySQL", ServiceName: "mongodb"}

	serviceInstanceCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	_, err = serviceInstanceCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	evt := createEvt(c)
	err = DeleteInstance(context.TODO(), si, evt, "")
	c.Assert(err, check.IsNil)
	query := mongoBSON.M{"name": si.Name}
	qtd, err := serviceInstanceCollection.CountDocuments(context.TODO(), query)
	c.Assert(err, check.IsNil)
	c.Assert(qtd, check.Equals, int64(0))
}

func (s *InstanceSuite) TestDeleteServiceInstanceWithForceRemoveEnabled(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	err := Create(context.TODO(), Service{
		Name:       "mongodb",
		Password:   "password",
		OwnerTeams: []string{"raul"},
		Endpoint: map[string]string{
			"production": ts.URL,
		},
	})
	c.Assert(err, check.IsNil)
	si := &ServiceInstance{Name: "MySQL", ServiceName: "mongodb"}

	serviceInstanceCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	_, err = serviceInstanceCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	evt := createEvt(c)
	si.ForceRemove = true
	err = DeleteInstance(context.TODO(), si, evt, "")
	c.Assert(err, check.IsNil)
	query := mongoBSON.M{"name": si.Name}
	qtd, err := serviceInstanceCollection.CountDocuments(context.TODO(), query)
	c.Assert(err, check.IsNil)
	c.Assert(qtd, check.Equals, int64(0))
}

func (s *InstanceSuite) TestFindApp(c *check.C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1", "app2"},
	}
	c.Assert(instance.FindApp("app1"), check.Equals, 0)
	c.Assert(instance.FindApp("app2"), check.Equals, 1)
	c.Assert(instance.FindApp("what"), check.Equals, -1)
}

func (s *InstanceSuite) TestBindApp(c *check.C) {
	oldBindAppDBAction := bindAppDBAction
	oldBindAppEndpointAction := bindAppEndpointAction
	oldSetBoundEnvsAction := setBoundEnvsAction
	defer func() {
		bindAppDBAction = oldBindAppDBAction
		bindAppEndpointAction = oldBindAppEndpointAction
		setBoundEnvsAction = oldSetBoundEnvsAction
	}()
	var calls []string
	var params []interface{}
	bindAppDBAction = &action.Action{
		Forward: func(ctx action.FWContext) (action.Result, error) {
			calls = append(calls, "bindAppDBAction")
			params = ctx.Params
			return nil, nil
		},
	}
	bindAppEndpointAction = &action.Action{
		Forward: func(ctx action.FWContext) (action.Result, error) {
			calls = append(calls, "bindAppEndpointAction")
			return nil, nil
		},
	}
	setBoundEnvsAction = &action.Action{
		Forward: func(ctx action.FWContext) (action.Result, error) {
			calls = append(calls, "setBoundEnvsAction")
			return nil, nil
		},
	}
	var si ServiceInstance
	a := provisiontest.NewFakeApp("myapp", "python", 1)
	var buf bytes.Buffer
	evt := createEvt(c)
	err := si.BindApp(context.TODO(), a, nil, true, &buf, evt, "")
	c.Assert(err, check.IsNil)
	expectedCalls := []string{
		"bindAppDBAction", "bindAppEndpointAction",
		"setBoundEnvsAction",
	}
	expectedParams := []interface{}{&bindAppPipelineArgs{
		app:             a,
		serviceInstance: &si,
		writer:          &buf,
		shouldRestart:   true,
		event:           evt,
		requestID:       "",
		params:          BindAppParameters(nil),
	}}
	c.Assert(calls, check.DeepEquals, expectedCalls)
	c.Assert(params, check.DeepEquals, expectedParams)
	c.Assert(buf.String(), check.Equals, "")
}

func (s *InstanceSuite) TestGetServiceInstancesBoundToApp(c *check.C) {
	srvc := Service{Name: "mysql"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	sInstance := ServiceInstance{
		Name:        "t3sql",
		ServiceName: "mysql",
		Tags:        []string{},
		Teams:       []string{s.team.Name},
		Apps:        []string{"app1", "app2"},
		Jobs:        []string{},
		Parameters:  map[string]interface{}{},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance)
	c.Assert(err, check.IsNil)
	sInstance2 := ServiceInstance{
		Name:        "s9sql",
		ServiceName: "mysql",
		Tags:        []string{},
		Apps:        []string{"app1"},
		Jobs:        []string{},
		Teams:       []string{},
		Parameters:  map[string]interface{}{},
	}

	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance2)
	c.Assert(err, check.IsNil)
	sInstances, err := GetServiceInstancesBoundToApp(context.TODO(), "app2")
	c.Assert(err, check.IsNil)
	expected := []ServiceInstance{sInstance}
	c.Assert(sInstances, check.DeepEquals, expected)
	sInstances, err = GetServiceInstancesBoundToApp(context.TODO(), "app1")
	c.Assert(err, check.IsNil)
	expected = []ServiceInstance{sInstance, sInstance2}
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServiceInstancesBoundToJob(c *check.C) {
	srvc := Service{Name: "mysql"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	sInstance := ServiceInstance{
		Name:        "j3sql",
		ServiceName: "mysql",
		Tags:        []string{},
		Teams:       []string{s.team.Name},
		Jobs:        []string{"job1", "job2"},
		Apps:        []string{},
		Parameters:  map[string]interface{}{},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance)
	c.Assert(err, check.IsNil)
	sInstance2 := ServiceInstance{
		Name:        "j9sql",
		ServiceName: "mysql",
		Tags:        []string{},
		Jobs:        []string{"job1"},
		Apps:        []string{},
		Teams:       []string{},
		Parameters:  map[string]interface{}{},
	}
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance2)
	c.Assert(err, check.IsNil)
	sInstances, err := GetServiceInstancesBoundToJob(context.TODO(), "job2")
	c.Assert(err, check.IsNil)
	expected := []ServiceInstance{sInstance}
	c.Assert(sInstances, check.DeepEquals, expected)
	sInstances, err = GetServiceInstancesBoundToJob(context.TODO(), "job1")
	c.Assert(err, check.IsNil)
	expected = []ServiceInstance{sInstance, sInstance2}
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServicesInstancesByTeamsAndNamesNoFilters(c *check.C) {
	sInstance1 := ServiceInstance{
		Name:        "t3sql",
		ServiceName: "mysql",
		Teams:       []string{"team1"},
		Tags:        []string{"tag1"},
		Apps:        []string{},
		Jobs:        []string{},
		Parameters:  map[string]interface{}{},
	}
	sInstance2 := ServiceInstance{
		Name:        "s9sql",
		ServiceName: "mysql",
		Teams:       []string{"team2"},
		Tags:        []string{"tag2"},
		Apps:        []string{},
		Jobs:        []string{},
		Parameters:  map[string]interface{}{},
	}

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance1)
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance2)
	c.Assert(err, check.IsNil)

	sInstances, err := GetServicesInstancesByTeamsAndNames(context.TODO(), nil, nil, "", "", nil)
	c.Assert(err, check.IsNil)

	expected := []ServiceInstance{sInstance1, sInstance2}

	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServicesInstancesByTeamsAndNamesFilteringByTeams(c *check.C) {
	sInstance1 := ServiceInstance{
		Name:        "t3sql",
		ServiceName: "mysql",
		Teams:       []string{"team1"},
		Tags:        []string{},
		Apps:        []string{},
		Jobs:        []string{},
		Parameters:  map[string]interface{}{},
	}
	sInstance2 := ServiceInstance{
		Name:        "s9sql",
		ServiceName: "mysql",
		Teams:       []string{"team2"},
		Tags:        []string{},
		Apps:        []string{},
		Jobs:        []string{},
		Parameters:  map[string]interface{}{},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance1)
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance2)
	c.Assert(err, check.IsNil)

	sInstances, err := GetServicesInstancesByTeamsAndNames(context.TODO(), []string{"team1"}, nil, "", "", nil)
	c.Assert(err, check.IsNil)

	expected := []ServiceInstance{sInstance1}
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServicesInstancesByTeamsAndNamesFilteringByNames(c *check.C) {
	sInstance1 := ServiceInstance{
		Name:        "t3sql",
		ServiceName: "mysql",
		Teams:       []string{"team1"},
		Tags:        []string{},
		Apps:        []string{},
		Jobs:        []string{},
		Parameters:  map[string]interface{}{},
	}
	sInstance2 := ServiceInstance{
		Name:        "s9sql",
		ServiceName: "mysql",
		Teams:       []string{"team2"},
		Tags:        []string{},
		Apps:        []string{},
		Jobs:        []string{},
		Parameters:  map[string]interface{}{},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance1)
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance2)
	c.Assert(err, check.IsNil)

	sInstances, err := GetServicesInstancesByTeamsAndNames(context.TODO(), nil, []string{"t3sql"}, "", "", nil)
	c.Assert(err, check.IsNil)

	expected := []ServiceInstance{sInstance1}
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServicesInstancesByTeamsAndNamesFilteringByTags(c *check.C) {
	sInstance1 := ServiceInstance{
		Name:        "t3sql",
		ServiceName: "mysql",
		Tags:        []string{"tag1", "tag2"},
		Teams:       []string{"team1"},
		Apps:        []string{},
		Jobs:        []string{},
		Parameters:  map[string]interface{}{},
	}
	sInstance2 := ServiceInstance{
		Name:        "s9sql",
		ServiceName: "mysql",
		Tags:        []string{"tag1", "tag3"},
		Teams:       []string{"team1"},
		Apps:        []string{},
		Jobs:        []string{},
		Parameters:  map[string]interface{}{},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance1)
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance2)
	c.Assert(err, check.IsNil)

	sInstances, err := GetServicesInstancesByTeamsAndNames(context.TODO(), nil, nil, "", "", []string{"tag1"})
	c.Assert(err, check.IsNil)

	expected := []ServiceInstance{sInstance1, sInstance2}
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServicesInstancesByTeamsAndNamesFilteringByMultipleCriteria(c *check.C) {
	sInstance1 := ServiceInstance{
		Name:        "t3sql",
		ServiceName: "mysql",
		Teams:       []string{"team1"},
		Tags:        []string{"tag1"},
		Apps:        []string{},
		Jobs:        []string{},
		Parameters:  map[string]interface{}{},
	}
	sInstance2 := ServiceInstance{
		Name:        "s9sql",
		ServiceName: "mysql",
		Teams:       []string{"team2"},
		Tags:        []string{"tag1", "tag3"},
		Apps:        []string{},
		Jobs:        []string{},
		Parameters:  map[string]interface{}{},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance1)
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance2)
	c.Assert(err, check.IsNil)

	sInstances, err := GetServicesInstancesByTeamsAndNames(context.TODO(), []string{"team2"}, []string{"s9sql"}, "", "", []string{"tag1"})
	c.Assert(err, check.IsNil)

	expected := []ServiceInstance{sInstance2}
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServiceInstancesByServices(c *check.C) {
	srvc := Service{Name: "mysql"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	sInstance := ServiceInstance{Name: "t3sql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance)
	c.Assert(err, check.IsNil)
	sInstance2 := ServiceInstance{Name: "s9sql", ServiceName: "mysql", Tags: []string{}}
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance2)
	c.Assert(err, check.IsNil)
	sInstances, err := GetServiceInstancesByServices(context.TODO(), []Service{srvc}, []string{})
	c.Assert(err, check.IsNil)
	expected := []ServiceInstance{{Name: "t3sql", ServiceName: "mysql", Tags: []string{}}, sInstance2}
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesWithoutAnyExistingServiceInstances(c *check.C) {
	srvc := Service{Name: "mysql"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	sInstances, err := GetServiceInstancesByServices(context.TODO(), []Service{srvc}, []string{})
	c.Assert(err, check.IsNil)
	c.Assert(sInstances, check.DeepEquals, []ServiceInstance(nil))
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesWithTwoServices(c *check.C) {
	srvc := Service{Name: "mysql"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	srvc2 := Service{Name: "mongodb"}

	_, err = servicesCollection.InsertOne(context.TODO(), &srvc2)
	c.Assert(err, check.IsNil)
	sInstance := ServiceInstance{Name: "t3sql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance)
	c.Assert(err, check.IsNil)
	sInstance2 := ServiceInstance{Name: "s9nosql", ServiceName: "mongodb", Tags: []string{"tag 1", "tag 2"}}
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance2)
	c.Assert(err, check.IsNil)
	sInstances, err := GetServiceInstancesByServices(context.TODO(), []Service{srvc, srvc2}, []string{})
	c.Assert(err, check.IsNil)
	expected := []ServiceInstance{{Name: "t3sql", ServiceName: "mysql", Tags: []string{}}, sInstance2}
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesFilteringByTags(c *check.C) {
	srvc := Service{Name: "mysql"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)

	sInstance1 := ServiceInstance{Name: "t3sql", ServiceName: "mysql", Tags: []string{"tag1", "tag2"}}
	sInstance2 := ServiceInstance{Name: "s9sql", ServiceName: "mysql", Tags: []string{"tag1", "tag3"}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance1)
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance2)
	c.Assert(err, check.IsNil)

	sInstances, err := GetServiceInstancesByServices(context.TODO(), []Service{srvc}, []string{"tag1"})
	c.Assert(err, check.IsNil)
	expected := []ServiceInstance{sInstance1, sInstance2}
	c.Assert(sInstances, check.DeepEquals, expected)

	sInstances, err = GetServiceInstancesByServices(context.TODO(), []Service{srvc}, []string{"tag1", "tag2"})
	c.Assert(err, check.IsNil)
	expected = []ServiceInstance{sInstance1}
	c.Assert(sInstances, check.DeepEquals, expected)

	sInstances, err = GetServiceInstancesByServices(context.TODO(), []Service{srvc}, []string{"tag4"})
	c.Assert(err, check.IsNil)
	c.Assert(sInstances, check.HasLen, 0)
}

func (s *InstanceSuite) TestGenericServiceInstancesFilter(c *check.C) {
	srvc := Service{Name: "mysql"}
	teams := []string{s.team.Name}
	query := genericServiceInstancesFilter(srvc, teams)
	c.Assert(query, check.DeepEquals, mongoBSON.M{"service_name": srvc.Name, "teams": mongoBSON.M{"$in": teams}})
}

func (s *InstanceSuite) TestGenericServiceInstancesFilterWithServiceSlice(c *check.C) {
	services := []Service{
		{Name: "mysql"},
		{Name: "mongodb"},
	}
	names := []string{"mysql", "mongodb"}
	teams := []string{s.team.Name}
	query := genericServiceInstancesFilter(services, teams)
	c.Assert(query, check.DeepEquals, mongoBSON.M{"service_name": mongoBSON.M{"$in": names}, "teams": mongoBSON.M{"$in": teams}})
}

func (s *InstanceSuite) TestGenericServiceInstancesFilterWithoutSpecifingTeams(c *check.C) {
	services := []Service{
		{Name: "mysql"},
		{Name: "mongodb"},
	}
	names := []string{"mysql", "mongodb"}
	teams := []string{}
	query := genericServiceInstancesFilter(services, teams)
	c.Assert(query, check.DeepEquals, mongoBSON.M{"service_name": mongoBSON.M{"$in": names}})
}

func (s *InstanceSuite) TestAdditionalInfo(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"label": "key", "value": "value"}, {"label": "key2", "value": "value2"}]`))
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	info, err := si.Info(context.TODO(), "")
	c.Assert(err, check.IsNil)
	expected := map[string]string{
		"key":  "value",
		"key2": "value2",
	}
	c.Assert(info, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestServiceInstanceInfoMarshalJSON(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"label": "key", "value": "value"}]`))
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	info, err := si.ToInfo(context.TODO())
	c.Assert(err, check.IsNil)
	data, err := json.Marshal(info)
	c.Assert(err, check.IsNil)
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"Id":          float64(0),
		"Name":        "ql",
		"PlanName":    "",
		"Teams":       nil,
		"Apps":        nil,
		"Jobs":        nil,
		"ServiceName": "mysql",
		"Info":        map[string]interface{}{"key": "value"},
		"TeamOwner":   "",
		"Pool":        "",
	}
	c.Assert(result, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestServiceInstanceInfoMarshalJSONWithoutInfo(c *check.C) {
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ""}}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	info, err := si.ToInfo(context.TODO())
	c.Assert(err, check.IsNil)
	data, err := json.Marshal(info)
	c.Assert(err, check.IsNil)
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"Id":          float64(0),
		"Name":        "ql",
		"PlanName":    "",
		"Teams":       nil,
		"Apps":        nil,
		"Jobs":        nil,
		"ServiceName": "mysql",
		"Info":        nil,
		"TeamOwner":   "",
		"Pool":        "",
	}
	c.Assert(result, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestServiceInstanceInfoMarshalJSONWithoutEndpoint(c *check.C) {
	srvc := Service{Name: "mysql"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	info, err := si.ToInfo(context.TODO())
	c.Assert(err, check.IsNil)
	data, err := json.Marshal(info)
	c.Assert(err, check.IsNil)
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"Id":          float64(0),
		"Name":        "ql",
		"PlanName":    "",
		"Teams":       nil,
		"Apps":        nil,
		"Jobs":        nil,
		"ServiceName": "mysql",
		"Info":        nil,
		"TeamOwner":   "",
		"Pool":        "",
	}
	c.Assert(result, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestDeleteInstance(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "instance", ServiceName: srv.Name}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	evt := createEvt(c)
	err = DeleteInstance(context.TODO(), &si, evt, "")
	h.Lock()
	defer h.Unlock()
	c.Assert(err, check.IsNil)
	l, err := serviceInstancesCollection.CountDocuments(context.TODO(), mongoBSON.M{"name": si.Name})
	c.Assert(err, check.IsNil)
	c.Assert(l, check.Equals, int64(0))
	c.Assert(h.url, check.Equals, "/resources/"+si.Name)
	c.Assert(h.method, check.Equals, "DELETE")
}

func (s *InstanceSuite) TestDeleteInstanceWithApps(c *check.C) {
	si := ServiceInstance{Name: "instance", Apps: []string{"foo"}}

	serviceInstanceCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	_, err = serviceInstanceCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	serviceInstanceCollection.DeleteOne(context.TODO(), mongoBSON.M{"name": si.Name})
	evt := createEvt(c)
	err = DeleteInstance(context.TODO(), &si, evt, "")
	c.Assert(err, check.ErrorMatches, "^This service instance is bound to at least one app. Unbind them before removing it$")
}

func (s *InstanceSuite) TestCreateServiceInstance(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "instance", PlanName: "small", TeamOwner: s.team.Name, Tags: []string{"tag1", "tag2"}}
	evt := createEvt(c)
	err = CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
	c.Assert(err, check.IsNil)
	si, err := GetServiceInstance(context.TODO(), "mongodb", "instance")
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
	c.Assert(si.PlanName, check.Equals, "small")
	c.Assert(si.TeamOwner, check.Equals, s.team.Name)
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(si.Tags, check.DeepEquals, []string{"tag1", "tag2"})
}

func (s *InstanceSuite) TestCreateServiceInstanceValidatesTeamOwner(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "instance", PlanName: "small", TeamOwner: "unknown", Tags: []string{"tag1", "tag2"}}
	evt := createEvt(c)
	err = CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
	c.Assert(err, check.ErrorMatches, "Team owner doesn't exist")
}

func (s *InstanceSuite) TestCreateServiceInstanceWithSameInstanceName(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := []Service{
		{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"},
		{Name: "mongodb2", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"},
		{Name: "mongodb3", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"},
	}
	instance := ServiceInstance{Name: "instance", PlanName: "small", TeamOwner: s.team.Name}
	evt := createEvt(c)
	for _, service := range srv {
		servicesCollection, err := storagev2.ServicesCollection()
		c.Assert(err, check.IsNil)
		_, err = servicesCollection.InsertOne(context.TODO(), &service)
		c.Assert(err, check.IsNil)
		err = CreateServiceInstance(context.TODO(), instance, &service, evt, "")
		c.Assert(err, check.IsNil)
	}
	si, err := GetServiceInstance(context.TODO(), "mongodb3", "instance")
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(3))
	c.Assert(si.PlanName, check.Equals, "small")
	c.Assert(si.TeamOwner, check.Equals, s.team.Name)
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(si.Name, check.Equals, "instance")
	c.Assert(si.ServiceName, check.Equals, "mongodb3")
	err = CreateServiceInstance(context.TODO(), instance, &srv[0], evt, "")
	c.Assert(err, check.Equals, ErrInstanceNameAlreadyExists)
}

func (s *InstanceSuite) TestCreateSpecifyOwner(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	team := authTypes.Team{Name: "owner"}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, team.Name)
		return &team, nil
	}
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "instance", PlanName: "small", TeamOwner: team.Name}
	evt := createEvt(c)
	err = CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
	c.Assert(err, check.IsNil)
	si, err := GetServiceInstance(context.TODO(), "mongodb", "instance")
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
	c.Assert(si.TeamOwner, check.Equals, team.Name)
}

func (s *InstanceSuite) TestCreateServiceInstanceNoTeamOwner(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "instance", PlanName: "small"}
	evt := createEvt(c)
	err = CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
	c.Assert(err, check.Equals, ErrTeamMandatory)
}

func (s *InstanceSuite) TestCreateServiceInstanceNameShouldBeUnique(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "instance", TeamOwner: s.team.Name}
	evt := createEvt(c)
	err = CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
	c.Assert(err, check.IsNil)
	err = CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
	c.Assert(err, check.Equals, ErrInstanceNameAlreadyExists)
}

func (s *InstanceSuite) TestCreateServiceInstanceEndpointFailure(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "instance"}
	evt := createEvt(c)
	err = CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
	c.Assert(err, check.NotNil)

	serviceInstanceCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	count, err := serviceInstanceCollection.CountDocuments(context.TODO(), mongoBSON.M{"name": "instance"})
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, int64(0))
}

func (s *InstanceSuite) TestCreateServiceInstanceValidatesTheName(c *check.C) {
	var tests = []struct {
		input string
		err   error
	}{
		{"my-service", nil},
		{"my_service", nil},
		{"MyService", nil},
		{"a1", nil},
		{"--app", ErrInvalidInstanceName},
		{"123servico", ErrInvalidInstanceName},
		{"a", ErrInvalidInstanceName},
		{"a@123", ErrInvalidInstanceName},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	evt := createEvt(c)
	for _, t := range tests {
		instance := ServiceInstance{Name: t.input, TeamOwner: s.team.Name}
		err := CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
		c.Check(err, check.Equals, t.err, check.Commentf(t.input))
	}
}

func (s *InstanceSuite) TestCreateServiceInstanceRemovesDuplicatedAndEmptyTags(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "instance", PlanName: "small", TeamOwner: s.team.Name, Tags: []string{"", "  tag1 ", "tag1", "  "}}
	evt := createEvt(c)
	err = CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
	c.Assert(err, check.IsNil)
	si, err := GetServiceInstance(context.TODO(), "mongodb", "instance")
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
	c.Assert(si.Tags, check.DeepEquals, []string{"tag1"})
}

func (s *InstanceSuite) TestCreateServiceInstanceMultiClusterInstanceWithoutPool(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Fail()
	}))
	defer ts.Close()
	srv := Service{
		Name:           "multicluster-service",
		Endpoint:       map[string]string{"production": ts.URL},
		Username:       "user",
		Password:       "password",
		IsMultiCluster: true,
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{
		Name:      "instance",
		TeamOwner: s.team.Name,
	}
	evt := createEvt(c)
	err = CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
	c.Assert(err, check.Equals, ErrMultiClusterServiceRequiresPool)
}

func (s *InstanceSuite) TestCreateServiceInstanceMultiClusterWhenPoolDoesNotExist(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Fail()
	}))
	defer ts.Close()
	s.mockService.Pool.OnFindByName = func(name string) (*provTypes.Pool, error) {
		return nil, provTypes.ErrPoolNotFound
	}
	srv := Service{
		Name:           "multicluster-service",
		Endpoint:       map[string]string{"production": ts.URL},
		Username:       "user",
		Password:       "password",
		IsMultiCluster: true,
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{
		Name:      "instance",
		TeamOwner: s.team.Name,
		Pool:      "not-found-pool",
	}
	evt := createEvt(c)
	err = CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "pool does not exist")
}

func (s *InstanceSuite) TestCreateServiceInstanceMultiClusterWhenNoClusterFound(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Header["X-Tsuru-Pool-Name"], check.DeepEquals, []string{"my-pool"})
		c.Assert(r.Header["X-Tsuru-Pool-Provisioner"], check.DeepEquals, []string{"docker"})
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	s.mockService.Pool.OnFindByName = func(name string) (*provTypes.Pool, error) {
		if name != "my-pool" {
			return nil, errors.New("pool not found")
		}
		return &provTypes.Pool{
			Name:        "my-pool",
			Provisioner: "docker",
		}, nil
	}
	s.mockService.Pool.OnServices = func(pool string) ([]string, error) {
		return []string{"multicluster-service"}, nil
	}
	s.mockService.Cluster.OnFindByPool = func(provisioner, name string) (*provTypes.Cluster, error) {
		return nil, provTypes.ErrNoCluster
	}
	srv := Service{
		Name:           "multicluster-service",
		Endpoint:       map[string]string{"production": ts.URL},
		Username:       "user",
		Password:       "password",
		IsMultiCluster: true,
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{
		Name:      "instance",
		TeamOwner: s.team.Name,
		Pool:      "my-pool",
	}
	err = CreateServiceInstance(context.TODO(), instance, &srv, createEvt(c), "")
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.Equals, int32(1))
}

func (s *InstanceSuite) TestCreateServiceInstanceMultiCluster(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Header["X-Tsuru-Pool-Name"], check.DeepEquals, []string{"my-pool"})
		c.Assert(r.Header["X-Tsuru-Pool-Provisioner"], check.DeepEquals, []string{"kubernetes"})
		c.Assert(r.Header["X-Tsuru-Cluster-Name"], check.DeepEquals, []string{"cluster-name"})
		c.Assert(r.Header["X-Tsuru-Cluster-Provisioner"], check.DeepEquals, []string{"kubernetes"})
		c.Assert(r.Header["X-Tsuru-Cluster-Addresses"], check.DeepEquals, []string{"https://my-kubernetes.example.com"})
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	s.mockService.Pool.OnFindByName = func(name string) (*provTypes.Pool, error) {
		return &provTypes.Pool{
			Name:        "my-pool",
			Provisioner: "kubernetes",
		}, nil
	}
	s.mockService.Cluster.OnFindByPool = func(provisioner, name string) (*provTypes.Cluster, error) {
		return &provTypes.Cluster{
			Name:        "cluster-name",
			Addresses:   []string{"https://my-kubernetes.example.com"},
			Provisioner: "kubernetes",
			Pools:       []string{"my-pool"},
		}, nil
	}
	s.mockService.Pool.OnServices = func(pool string) ([]string, error) {
		return []string{"multicluster-service"}, nil
	}
	srv := Service{
		Name:           "multicluster-service",
		Endpoint:       map[string]string{"production": ts.URL},
		Username:       "user",
		Password:       "password",
		IsMultiCluster: true,
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{
		Name:      "instance",
		TeamOwner: s.team.Name,
		Pool:      "my-pool",
	}
	err = CreateServiceInstance(context.TODO(), instance, &srv, createEvt(c), "")
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.Equals, int32(1))
}

func (s *InstanceSuite) TestCreateServiceInstanceMultiClusterWithKubeConfig(c *check.C) {
	var requests int32

	kubeConfig := &provTypes.KubeConfig{
		Cluster: clientcmdapi.Cluster{
			Server: "https://mycluster-from-kubeconfig.com",
		},
		AuthInfo: clientcmdapi.AuthInfo{
			Token: "my-token-from-kubeconfig",
		},
	}

	kubeConfigJSON, err := json.Marshal(kubeConfig)
	c.Assert(err, check.IsNil)
	kubeConfigBase64 := base64.StdEncoding.EncodeToString(kubeConfigJSON)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Header["X-Tsuru-Pool-Name"], check.DeepEquals, []string{"my-pool"})
		c.Assert(r.Header["X-Tsuru-Pool-Provisioner"], check.DeepEquals, []string{"kubernetes"})
		c.Assert(r.Header["X-Tsuru-Cluster-Name"], check.DeepEquals, []string{"cluster-name"})
		c.Assert(r.Header["X-Tsuru-Cluster-Provisioner"], check.DeepEquals, []string{"kubernetes"})
		c.Assert(r.Header["X-Tsuru-Cluster-Addresses"], check.DeepEquals, []string{"https://my-kubernetes.example.com"})
		c.Assert(r.Header["X-Tsuru-Cluster-Kube-Config"], check.DeepEquals, []string{kubeConfigBase64})

		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	s.mockService.Pool.OnFindByName = func(name string) (*provTypes.Pool, error) {
		return &provTypes.Pool{
			Name:        "my-pool",
			Provisioner: "kubernetes",
		}, nil
	}
	s.mockService.Cluster.OnFindByPool = func(provisioner, name string) (*provTypes.Cluster, error) {
		return &provTypes.Cluster{
			Name:        "cluster-name",
			Addresses:   []string{"https://my-kubernetes.example.com"},
			Provisioner: "kubernetes",
			Pools:       []string{"my-pool"},
			CustomData: map[string]string{
				"propagate-kubeconfig": "true",
			},
			KubeConfig: kubeConfig,
		}, nil
	}
	s.mockService.Pool.OnServices = func(pool string) ([]string, error) {
		return []string{"multicluster-service"}, nil
	}
	srv := Service{
		Name:           "multicluster-service",
		Endpoint:       map[string]string{"production": ts.URL},
		Username:       "user",
		Password:       "password",
		IsMultiCluster: true,
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{
		Name:      "instance",
		TeamOwner: s.team.Name,
		Pool:      "my-pool",
	}
	err = CreateServiceInstance(context.TODO(), instance, &srv, createEvt(c), "")
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.Equals, int32(1))
}

func (s *InstanceSuite) TestCreateServiceInstanceRegularServiceWithPool(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Fail()
	}))
	defer ts.Close()
	srv := &Service{
		Name:           "non-multi-cluster-service",
		Endpoint:       map[string]string{"production": ts.URL},
		Username:       "user",
		Password:       "password",
		IsMultiCluster: false,
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), srv)
	c.Assert(err, check.IsNil)
	err = CreateServiceInstance(context.TODO(), ServiceInstance{
		Name:        "my-instance",
		ServiceName: "non-multi-cluster-service",
		TeamOwner:   s.team.Name,
		Pool:        "my-pool",
	}, srv, createEvt(c), "")
	c.Assert(err, check.Equals, ErrRegularServiceInstanceCannotBelongToPool)
}

func (s *InstanceSuite) TestUpdateServiceInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "instance", ServiceName: "mongodb", PlanName: "small", TeamOwner: s.team.Name, Tags: []string{"tag1"}, Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	var si ServiceInstance
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": "instance"}).Decode(&si)
	c.Assert(err, check.IsNil)
	newTeam := authTypes.Team{Name: "new-team-owner"}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, newTeam.Name)
		return &newTeam, nil
	}
	si.Description = "desc"
	si.Tags = []string{"tag2"}
	si.TeamOwner = newTeam.Name
	evt := createEvt(c)
	err = instance.Update(context.TODO(), srv, si, evt, "")
	c.Assert(err, check.IsNil)
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": "instance"}).Decode(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.PlanName, check.Equals, "small")
	c.Assert(si.Description, check.Equals, "desc")
	c.Assert(si.Tags, check.DeepEquals, []string{"tag2"})
	c.Assert(si.TeamOwner, check.Equals, newTeam.Name)
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name, newTeam.Name})
}

func (s *InstanceSuite) TestUpdateServiceInstanceValidatesTeamOwner(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "instance", ServiceName: "mongodb", PlanName: "small", TeamOwner: s.team.Name, Tags: []string{"tag1"}}
	evt := createEvt(c)
	err = CreateServiceInstance(context.TODO(), instance, &srv, evt, "")
	c.Assert(err, check.IsNil)
	var si ServiceInstance

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": "instance"}).Decode(&si)
	c.Assert(err, check.IsNil)

	si.TeamOwner = "unknown"
	err = instance.Update(context.TODO(), srv, si, evt, "")
	c.Assert(err, check.ErrorMatches, "Team owner doesn't exist")
}

func (s *InstanceSuite) TestUpdateServiceInstanceRemovesDuplicatedAndEmptyTags(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "instance", ServiceName: "mongodb", PlanName: "small", TeamOwner: s.team.Name, Tags: []string{"tag1"}, Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	instance.Tags = []string{"tag2", " ", " tag2 "}
	evt := createEvt(c)
	err = instance.Update(context.TODO(), srv, instance, evt, "")
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
	var si ServiceInstance
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": "instance"}).Decode(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Tags, check.DeepEquals, []string{"tag2"})
}

func (s *InstanceSuite) TestStatus(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "instance", ServiceName: srv.Name}
	status, err := si.Status(context.TODO(), "")
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, "up")
}

func (s *InstanceSuite) TestGetServiceInstance(c *check.C) {
	serviceInstanceCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	serviceInstanceCollection.InsertMany(context.TODO(), []interface{}{
		ServiceInstance{Name: "mongo-1", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-2", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-3", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-4", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-5", ServiceName: "mongodb"},
	})
	instance, err := GetServiceInstance(context.TODO(), "mongodb", "mongo-1")
	c.Assert(err, check.IsNil)
	c.Assert(instance.Name, check.Equals, "mongo-1")
	c.Assert(instance.ServiceName, check.Equals, "mongodb")
	c.Assert(instance.Teams, check.DeepEquals, []string{s.team.Name})
	instance, err = GetServiceInstance(context.TODO(), "mongodb", "mongo-6")
	c.Assert(instance, check.IsNil)
	c.Assert(err, check.Equals, ErrServiceInstanceNotFound)
	instance, err = GetServiceInstance(context.TODO(), "mongodb", "mongo-5")
	c.Assert(err, check.IsNil)
	c.Assert(instance.Name, check.Equals, "mongo-5")
}

func (s *InstanceSuite) TestGetIdentfier(c *check.C) {
	srv := ServiceInstance{Name: "mongodb"}
	identifier := srv.GetIdentifier()
	c.Assert(identifier, check.Equals, srv.Name)
	srv.Id = 10
	identifier = srv.GetIdentifier()
	c.Assert(identifier, check.Equals, strconv.Itoa(srv.Id))
}

func (s *InstanceSuite) TestGrantTeamToInstance(c *check.C) {
	user := &auth.User{Email: "test@raul.com", Password: "123"}

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	_, err = usersCollection.InsertOne(context.TODO(), user)
	c.Assert(err, check.IsNil)
	team := authTypes.Team{Name: "test2"}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, team.Name)
		return &team, nil
	}
	srvc := Service{Name: "mysql", Teams: []string{team.Name}, IsRestricted: false}
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	sInstance := ServiceInstance{
		Name:        "j4sql",
		ServiceName: srvc.Name,
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance)
	c.Assert(err, check.IsNil)
	sInstance.Grant(context.TODO(), team.Name)
	si, err := GetServiceInstance(context.TODO(), "mysql", "j4sql")
	c.Assert(err, check.IsNil)
	c.Assert(si.Teams, check.DeepEquals, []string{"test2"})
}

func (s *InstanceSuite) TestRevokeTeamToInstance(c *check.C) {
	user := &auth.User{Email: "test@raul.com", Password: "123"}

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	_, err = usersCollection.InsertOne(context.TODO(), user)
	c.Assert(err, check.IsNil)
	team := authTypes.Team{Name: "test2"}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, team.Name)
		return &team, nil
	}
	srvc := Service{Name: "mysql", Teams: []string{team.Name}, IsRestricted: false}
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	sInstance := ServiceInstance{
		Name:        "j4sql",
		ServiceName: srvc.Name,
		Teams:       []string{team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance)
	c.Assert(err, check.IsNil)
	si, err := GetServiceInstance(context.TODO(), "mysql", "j4sql")
	c.Assert(err, check.IsNil)
	c.Assert(si.Teams, check.DeepEquals, []string{"test2"})
	sInstance.Revoke(context.TODO(), team.Name)
	si, err = GetServiceInstance(context.TODO(), "mysql", "j4sql")
	c.Assert(err, check.IsNil)
	c.Assert(si.Teams, check.DeepEquals, []string{})
}

func (s *InstanceSuite) TestRevokeTeamOwner(c *check.C) {
	user := &auth.User{Email: "user@tsuru.io", Password: "12345"}

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	_, err = usersCollection.InsertOne(context.TODO(), user)
	c.Assert(err, check.IsNil)
	team := authTypes.Team{Name: "team-one"}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		return nil, fmt.Errorf("should not pass here")
	}
	srvc := Service{Name: "service-one", Teams: []string{team.Name}}
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &ServiceInstance{
		Name:        "instance-one",
		ServiceName: srvc.Name,
		TeamOwner:   "team-one",
		Teams:       []string{team.Name},
	})
	c.Assert(err, check.IsNil)
	si, err := GetServiceInstance(context.TODO(), "service-one", "instance-one")
	c.Assert(err, check.IsNil)
	err = si.Revoke(context.TODO(), team.Name)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "cannot revoke the instance's team owner access")
}

func (s *InstanceSuite) TestUnbindApp(c *check.C) {
	var reqs []*http.Request
	var mut sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	serv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := Create(context.TODO(), serv)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 2)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	err = servicemanager.App.AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "ENV1", Value: "VAL1"}, ServiceName: "mysql", InstanceName: "my-mysql"},
			{EnvVar: bindTypes.EnvVar{Name: "ENV2", Value: "VAL2"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	evt := createEvt(c)
	evt.SetLogWriter(&buf)
	err = si.UnbindApp(context.TODO(), UnbindAppArgs{
		App:   a,
		Event: evt,
	})
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Matches, ".*remove instance")
	c.Assert(reqs, check.HasLen, 1)
	c.Assert(reqs[0].Method, check.Equals, "DELETE")
	c.Assert(reqs[0].URL.Path, check.Equals, "/resources/my-mysql/bind-app")
	siDB, err := GetServiceInstance(context.TODO(), "mysql", si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.Apps, check.DeepEquals, []string{})
	c.Assert(a.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{})
}

func (s *InstanceSuite) TestUnbindAppFailureInUnbindAppCall(c *check.C) {
	var reqs []*http.Request
	var mut sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()
		reqs = append(reqs, r)
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind-app" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("my unbind app err"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	serv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := Create(context.TODO(), serv)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 2)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	err = servicemanager.App.AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "ENV1", Value: "VAL1"}, ServiceName: "mysql", InstanceName: "my-mysql"},
			{EnvVar: bindTypes.EnvVar{Name: "ENV2", Value: "VAL2"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	evt := createEvt(c)
	evt.SetLogWriter(&buf)
	err = si.UnbindApp(context.TODO(), UnbindAppArgs{
		App:     a,
		Restart: true,
		Event:   evt,
	})
	c.Assert(err, check.ErrorMatches, `Failed to unbind \("/resources/my-mysql/bind-app"\): invalid response: my unbind app err \(code: 500\)`)
	c.Assert(buf.String(), check.Matches, "")
	c.Assert(si.Apps, check.DeepEquals, []string{"myapp"})
	c.Assert(reqs, check.HasLen, 1)
	c.Assert(reqs[0].Method, check.Equals, "DELETE")
	c.Assert(reqs[0].URL.Path, check.Equals, "/resources/my-mysql/bind-app")
	siDB, err := GetServiceInstance(context.TODO(), si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.Apps, check.DeepEquals, []string{"myapp"})
	c.Assert(a.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "ENV1", Value: "VAL1"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		{EnvVar: bindTypes.EnvVar{Name: "ENV2", Value: "VAL2"}, ServiceName: "mysql", InstanceName: "my-mysql"},
	})
}

func (s *InstanceSuite) TestUnbindAppFailureInUnbindAppCallWithForce(c *check.C) {
	var reqs []*http.Request
	var mut sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()
		reqs = append(reqs, r)
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind-app" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("my unbind app err"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	serv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := Create(context.TODO(), serv)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 2)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	err = servicemanager.App.AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "ENV1", Value: "VAL1"}, ServiceName: "mysql", InstanceName: "my-mysql"},
			{EnvVar: bindTypes.EnvVar{Name: "ENV2", Value: "VAL2"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	evt := createEvt(c)
	evt.SetLogWriter(&buf)
	err = si.UnbindApp(context.TODO(), UnbindAppArgs{
		App:         a,
		Restart:     true,
		ForceRemove: true,
		Event:       evt,
	})
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Matches, `(?s).*\[unbind-app-endpoint\] ignored error due to force: Failed to unbind \("/resources/my-mysql/bind-app"\): invalid response: my unbind app err \(code: 500\).*remove instance`)
	c.Assert(reqs, check.HasLen, 1)
	c.Assert(reqs[0].Method, check.Equals, "DELETE")
	c.Assert(reqs[0].URL.Path, check.Equals, "/resources/my-mysql/bind-app")
	siDB, err := GetServiceInstance(context.TODO(), si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.Apps, check.DeepEquals, []string{})
	c.Assert(a.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{})
}

func (s *InstanceSuite) TestUnbindAppFailureInAppEnvSet(c *check.C) {
	var reqs []*http.Request
	var mut sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	serv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := Create(context.TODO(), serv)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 2)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	evt := createEvt(c)
	evt.SetLogWriter(&buf)
	err = si.UnbindApp(context.TODO(), UnbindAppArgs{
		App:     a,
		Restart: true,
		Event:   evt,
	})
	c.Assert(err, check.ErrorMatches, `instance not found`)
	c.Assert(buf.String(), check.Matches, "")
	c.Assert(si.Apps, check.DeepEquals, []string{"myapp"})
	c.Assert(reqs, check.HasLen, 2)
	c.Assert(reqs[0].Method, check.Equals, "DELETE")
	c.Assert(reqs[0].URL.Path, check.Equals, "/resources/my-mysql/bind-app")
	c.Assert(reqs[1].Method, check.Equals, "POST")
	c.Assert(reqs[1].URL.Path, check.Equals, "/resources/my-mysql/bind-app")
	siDB, err := GetServiceInstance(context.TODO(), si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.Apps, check.DeepEquals, []string{"myapp"})
}

func (s *InstanceSuite) TestBindAppFullPipeline(c *check.C) {
	var reqs []*http.Request
	var mut sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/resources/my-mysql/bind-app" && r.Method == "POST" {
			w.Write([]byte(`{"ENV1": "VAL1", "ENV2": "VAL2"}`))
		}
	}))
	defer ts.Close()
	serv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := Create(context.TODO(), serv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 2)
	var buf bytes.Buffer
	evt := createEvt(c)
	err = si.BindApp(context.TODO(), a, nil, true, &buf, evt, "")
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Matches, "add instance")
	c.Assert(reqs, check.HasLen, 1)
	c.Assert(reqs[0].Method, check.Equals, "POST")
	c.Assert(reqs[0].URL.Path, check.Equals, "/resources/my-mysql/bind-app")
	siDB, err := GetServiceInstance(context.TODO(), si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.Apps, check.DeepEquals, []string{"myapp"})
	c.Assert(a.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "ENV1", Value: "VAL1"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		{EnvVar: bindTypes.EnvVar{Name: "ENV2", Value: "VAL2"}, ServiceName: "mysql", InstanceName: "my-mysql"},
	})
}

func (s *InstanceSuite) TestBindAppMultipleApps(c *check.C) {
	goMaxProcs := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(goMaxProcs)
	var reqs []*http.Request
	var mut sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/resources/my-mysql/bind-app" && r.Method == "POST" {
			w.Write([]byte(`{"ENV1": "VAL1", "ENV2": "VAL2"}`))
		}
	}))
	defer ts.Close()
	serv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := Create(context.TODO(), serv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	var apps []*appTypes.App
	var expectedNames []string
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("myapp-%02d", i)
		expectedNames = append(expectedNames, name)
		apps = append(apps, provisiontest.NewFakeApp(name, "static", 2))
	}
	evt := createEvt(c)
	wg := sync.WaitGroup{}
	for _, app := range apps {
		wg.Add(1)
		go func(app *appTypes.App) {
			defer wg.Done()
			var buf bytes.Buffer
			bindErr := si.BindApp(context.TODO(), app, nil, true, &buf, evt, "")
			c.Assert(bindErr, check.IsNil)
		}(app)
	}
	wg.Wait()
	c.Assert(reqs, check.HasLen, 100)
	siDB, err := GetServiceInstance(context.TODO(), si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	sort.Strings(siDB.Apps)
	c.Assert(siDB.Apps, check.DeepEquals, expectedNames)
}

func (s *InstanceSuite) TestUnbindAppMultipleApps(c *check.C) {
	originalMaxProcs := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	var reqs []*http.Request
	var mut sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/resources/my-mysql/bind-app" && r.Method == "POST" {
			w.Write([]byte(`{"ENV1": "VAL1", "ENV2": "VAL2"}`))
		}
	}))
	defer ts.Close()
	serv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := Create(context.TODO(), serv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	var apps []*appTypes.App
	evt := createEvt(c)
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("myapp-%02d", i)
		app := provisiontest.NewFakeApp(name, "static", 2)
		apps = append(apps, app)
		var buf bytes.Buffer
		err = si.BindApp(context.TODO(), app, nil, true, &buf, evt, "")
		c.Assert(err, check.IsNil)
	}
	siDB, err := GetServiceInstance(context.TODO(), si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	wg := sync.WaitGroup{}
	for _, app := range apps {
		wg.Add(1)
		go func(app *appTypes.App) {
			defer wg.Done()
			unbindErr := siDB.UnbindApp(context.TODO(), UnbindAppArgs{
				App:   app,
				Event: evt,
			})
			c.Assert(unbindErr, check.IsNil)
		}(app)
	}
	wg.Wait()
	c.Assert(len(reqs), check.Equals, 40)
	siDB, err = GetServiceInstance(context.TODO(), si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	sort.Strings(siDB.Apps)
	c.Assert(siDB.Apps, check.DeepEquals, []string{})
}

func (s *S) TestRenameServiceInstanceTeam(c *check.C) {
	sInstances := []any{
		ServiceInstance{Name: "si1", ServiceName: "mysql", Teams: []string{"team1", "team2", "team3"}, TeamOwner: "team1", Parameters: map[string]interface{}{}},
		ServiceInstance{Name: "si2", ServiceName: "mysql", Teams: []string{"team1", "team3"}, TeamOwner: "team2", Parameters: map[string]interface{}{}},
		ServiceInstance{Name: "si3", ServiceName: "mysql", Teams: []string{"team2", "team3"}, TeamOwner: "team3", Parameters: map[string]interface{}{}},
	}

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	_, err = serviceInstancesCollection.InsertMany(context.TODO(), sInstances)

	c.Assert(err, check.IsNil)

	err = RenameServiceInstanceTeam(context.TODO(), "team2", "team9000")
	c.Assert(err, check.IsNil)
	var dbInstances []ServiceInstance

	cursor, err := serviceInstancesCollection.Find(context.TODO(), mongoBSON.M{}, &options.FindOptions{
		Sort: mongoBSON.M{"name": 1},
	})
	c.Assert(err, check.IsNil)

	err = cursor.All(context.TODO(), &dbInstances)
	c.Assert(err, check.IsNil)

	c.Assert(dbInstances, check.DeepEquals, []ServiceInstance{
		{Name: "si1", ServiceName: "mysql", Teams: []string{"team1", "team3", "team9000"}, TeamOwner: "team1", Apps: []string{}, Jobs: []string{}, Tags: []string{}, Parameters: map[string]interface{}{}},
		{Name: "si2", ServiceName: "mysql", Teams: []string{"team1", "team3"}, TeamOwner: "team9000", Apps: []string{}, Jobs: []string{}, Tags: []string{}, Parameters: map[string]interface{}{}},
		{Name: "si3", ServiceName: "mysql", Teams: []string{"team3", "team9000"}, TeamOwner: "team3", Apps: []string{}, Jobs: []string{}, Tags: []string{}, Parameters: map[string]interface{}{}},
	})
}

func (s *S) TestProxyInstance(c *check.C) {
	var remoteReq *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteReq = r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	service := Service{
		Name:       "tensorflow",
		Endpoint:   map[string]string{"production": ts.URL},
		Password:   "abcde",
		OwnerTeams: []string{s.team.Name},
	}
	err := Create(context.TODO(), service)
	c.Assert(err, check.IsNil)
	sInstance := ServiceInstance{Name: "noflow", ServiceName: "tensorflow", Teams: []string{s.team.Name}}

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &sInstance)
	c.Assert(err, check.IsNil)
	tests := []struct {
		method       string
		path         string
		expectedPath string
		err          string
	}{
		{method: "GET", path: "", expectedPath: "/resources/noflow"},
		{method: "GET", path: "/", expectedPath: "/resources/noflow"},
		{method: "GET", path: "/resources/noflow", expectedPath: "/resources/noflow"},
		{method: "GET", path: "/resources/noflow/", expectedPath: "/resources/noflow"},
		{method: "GET", path: "/resources/noflowxxx", expectedPath: "/resources/noflow/resources/noflowxxx"},
		{method: "POST", path: "", err: "proxy request POST \"\" is forbidden"},
		{method: "POST", path: "bind-app", err: "proxy request POST \"bind-app\" is forbidden"},
		{method: "POST", path: "/bind-app", err: "proxy request POST \"bind-app\" is forbidden"},
		{method: "GET", path: "/bind-app", expectedPath: "/resources/noflow/bind-app"},
		{method: "GET", path: "/resources/noflow/bind-app", expectedPath: "/resources/noflow/bind-app"},
		{method: "POST", path: "/resources/noflow/otherpath", expectedPath: "/resources/noflow/otherpath"},
		{method: "POST", path: "/resources/otherinstance/otherpath", expectedPath: "/resources/noflow/resources/otherinstance/otherpath"},
		// Path traversal attempts - should be blocked
		{method: "GET", path: "/resources/noflow/../otherinstance/secret", err: "invalid proxy path"},
	}
	evt := createEvt(c)
	for _, tt := range tests {
		request, err := http.NewRequest(tt.method, "", nil)
		c.Assert(err, check.IsNil)
		recorder := httptest.NewRecorder()
		err = ProxyInstance(context.TODO(), &sInstance, tt.path, evt, "", recorder, request)
		if tt.err == "" {
			c.Assert(err, check.IsNil)
			c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
			c.Assert(remoteReq.URL.Path, check.Equals, tt.expectedPath)
		} else {
			c.Assert(err, check.ErrorMatches, tt.err)
		}
	}
}
