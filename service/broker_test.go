package service

import (
	"context"
	"encoding/json"
	"sort"

	osb "github.com/pmorie/go-open-service-broker-client/v2"
	osbfake "github.com/pmorie/go-open-service-broker-client/v2/fake"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/provision/provisiontest"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	check "gopkg.in/check.v1"
)

func (s *S) TestBrokerClientPlans(c *check.C) {
	config := osbfake.FakeClientConfiguration{
		CatalogReaction: &osbfake.CatalogReaction{Response: &osb.CatalogResponse{
			Services: []osb.Service{
				{Name: "otherservice"},
				{
					Name:        "service",
					Description: "This service is awesome!",
					Plans: []osb.Plan{
						{Name: "plan1", Description: "First plan"},
						{Name: "plan2", Description: "Second plan"},
					},
				},
			},
		}},
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	client, err := newClient(serviceTypes.Broker{Name: "broker"}, "service")
	c.Assert(err, check.IsNil)
	plans, err := client.Plans(context.TODO(), "", "")
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.DeepEquals, []Plan{
		{Name: "plan1", Description: "First plan"},
		{Name: "plan2", Description: "Second plan"},
	})
}

func (s *S) TestBrokerClientCreate(c *check.C) {
	var provisioned bool
	ev := createEvt(c)
	reaction := func(req *osb.ProvisionRequest) (*osb.ProvisionResponse, error) {
		provisioned = true
		exID, err := json.Marshal(map[string]interface{}{
			"user": "my@user",
		})
		c.Assert(err, check.IsNil)
		orgID := req.OrganizationGUID
		c.Assert(req.InstanceID, check.Not(check.DeepEquals), "")
		c.Assert(req.OrganizationGUID, check.Not(check.DeepEquals), "")
		c.Assert(req.SpaceGUID, check.Not(check.DeepEquals), "")
		req.InstanceID = ""
		req.OrganizationGUID = ""
		req.SpaceGUID = ""
		c.Assert(req, check.DeepEquals, &osb.ProvisionRequest{
			PlanID:    "planid",
			ServiceID: "serviceid",
			OriginatingIdentity: &osb.OriginatingIdentity{
				Platform: "tsuru",
				Value:    string(exID),
			},
			Context: map[string]interface{}{
				"request_id":        "request-id",
				"event_id":          ev.UniqueID.Hex(),
				"organization_guid": orgID,
				"space_guid":        orgID,
				"Namespace":         "broker-namespace",
			},
			AcceptsIncomplete: true,
		})
		return nil, nil
	}
	config := osbfake.FakeClientConfiguration{
		ProvisionReaction: osbfake.DynamicProvisionReaction(reaction),
		CatalogReaction: &osbfake.CatalogReaction{
			Response: &osb.CatalogResponse{
				Services: []osb.Service{
					{Name: "service", ID: "serviceid", Plans: []osb.Plan{{Name: "plan1", ID: "planid"}}},
				},
			},
		},
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	client, err := newClient(serviceTypes.Broker{
		Name: "broker",
		Config: serviceTypes.BrokerConfig{
			Context: map[string]interface{}{
				"Namespace": "broker-namespace",
			},
		},
	}, "service")
	c.Assert(err, check.IsNil)
	instance := createTestInstance()
	err = client.Create(context.TODO(), &instance, ev, "request-id")
	c.Assert(err, check.IsNil)
	c.Assert(provisioned, check.DeepEquals, true)
	provisioned = false
	instance.PlanName = "premium"
	err = client.Create(context.TODO(), &instance, ev, "request-id")
	c.Assert(err, check.ErrorMatches, `invalid plan: premium`)
	c.Assert(provisioned, check.DeepEquals, false)
}

func (s *S) TestBrokerClientStatus(c *check.C) {
	reaction := func(req *osb.LastOperationRequest) (*osb.LastOperationResponse, error) {
		pID := "p2"
		sID := "s1"
		exID, err := json.Marshal(map[string]interface{}{
			"team": "teamOwner",
		})
		c.Assert(err, check.IsNil)
		opKey := osb.OperationKey("")
		c.Assert(req, check.DeepEquals, &osb.LastOperationRequest{
			InstanceID: "e7252f14-54be-45df-bd40-e988a0e41059",
			ServiceID:  &sID,
			PlanID:     &pID,
			OriginatingIdentity: &osb.OriginatingIdentity{
				Platform: "tsuru",
				Value:    string(exID),
			},
			OperationKey: &opKey,
		})
		alldone := "last operation done!"
		return &osb.LastOperationResponse{
			State:       osb.StateSucceeded,
			Description: &alldone,
		}, nil
	}
	config := osbfake.FakeClientConfiguration{
		CatalogReaction: &osbfake.CatalogReaction{Response: &osb.CatalogResponse{
			Services: []osb.Service{
				{
					ID:   "s1",
					Name: "service",
					Plans: []osb.Plan{
						{ID: "p1", Name: "plan1", Description: "First plan"},
						{ID: "p2", Name: "plan2", Description: "Second plan"},
					},
				},
			},
		}},
		PollLastOperationReaction: osbfake.DynamicPollLastOperationReaction(reaction),
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	client, err := newClient(serviceTypes.Broker{Name: "broker"}, "service")
	c.Assert(err, check.IsNil)
	instance := createTestInstance()
	instance.PlanName = "plan2"
	instance.BrokerData.PlanID = "p2"
	status, err := client.Status(context.TODO(), &instance, "req-id")
	c.Assert(err, check.IsNil)
	c.Assert(status, check.DeepEquals, "succeeded - last operation done!")
}

func (s *S) TestBrokerClientDestroy(c *check.C) {
	var calls int
	ev := createEvt(c)
	reaction := func(req *osb.DeprovisionRequest) (*osb.DeprovisionResponse, error) {
		calls++
		exID, err := json.Marshal(map[string]interface{}{
			"user": "my@user",
		})
		c.Assert(err, check.IsNil)
		c.Assert(req, check.DeepEquals, &osb.DeprovisionRequest{
			InstanceID: "e7252f14-54be-45df-bd40-e988a0e41059",
			ServiceID:  "s1",
			PlanID:     "p1",
			OriginatingIdentity: &osb.OriginatingIdentity{
				Platform: "tsuru",
				Value:    string(exID),
			},
			AcceptsIncomplete: true,
		})
		return nil, nil
	}
	config := osbfake.FakeClientConfiguration{
		CatalogReaction: &osbfake.CatalogReaction{Response: &osb.CatalogResponse{
			Services: []osb.Service{
				{
					ID:   "s1",
					Name: "service",
					Plans: []osb.Plan{
						{ID: "p1", Name: "plan1", Description: "First plan"},
					},
				},
			},
		}},
		DeprovisionReaction: osbfake.DynamicDeprovisionReaction(reaction),
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	client, err := newClient(serviceTypes.Broker{Name: "broker"}, "service")
	c.Assert(err, check.IsNil)
	instance := createTestInstance()
	err = client.Destroy(context.TODO(), &instance, ev, "req-id")
	c.Assert(err, check.IsNil)
	c.Assert(calls, check.DeepEquals, 1)
}

func (s *S) TestBrokerClientBindApp(c *check.C) {
	ev := createEvt(c)
	a := provisiontest.NewFakeApp("theapp", "python", 1)
	appGUID, err := a.GetUUID(context.TODO())
	c.Assert(err, check.IsNil)
	var bindID string
	reaction := func(req *osb.BindRequest) (*osb.BindResponse, error) {
		exID, errMarshal := json.Marshal(map[string]interface{}{
			"user": "my@user",
		})
		c.Assert(errMarshal, check.IsNil)
		c.Assert(req.BindingID, check.Not(check.DeepEquals), "")
		bindID = req.BindingID
		req.BindingID = ""
		c.Assert(req, check.DeepEquals, &osb.BindRequest{
			AcceptsIncomplete: true,
			InstanceID:        "e7252f14-54be-45df-bd40-e988a0e41059",
			ServiceID:         "s1",
			PlanID:            "p1",
			AppGUID:           &appGUID,
			BindResource: &osb.BindResource{
				AppGUID: &appGUID,
			},
			OriginatingIdentity: &osb.OriginatingIdentity{
				Platform: "tsuru",
				Value:    string(exID),
			},
			Context: map[string]interface{}{
				"request_id": "request-id",
				"event_id":   ev.UniqueID.Hex(),
				"Namespace":  "broker-namespace",
			},
			Parameters: map[string]interface{}{
				"param1": "val1",
			},
		})
		opKey := osb.OperationKey("Binding")
		return &osb.BindResponse{
			OperationKey: &opKey,
			Credentials: map[string]interface{}{
				"env1": "val1",
				"env2": "val2",
				"env3": 3,
			}}, nil
	}
	config := osbfake.FakeClientConfiguration{
		CatalogReaction: &osbfake.CatalogReaction{Response: &osb.CatalogResponse{
			Services: []osb.Service{
				{
					ID:   "s1",
					Name: "service",
					Plans: []osb.Plan{
						{ID: "p1", Name: "plan1", Description: "First plan"},
					},
				},
			},
		}},
		BindReaction: osbfake.DynamicBindReaction(reaction),
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	client, err := newClient(serviceTypes.Broker{
		Name: "broker",
		Config: serviceTypes.BrokerConfig{
			Context: map[string]interface{}{
				"Namespace": "broker-namespace",
			},
		},
	}, "service")
	c.Assert(err, check.IsNil)
	instance := createTestInstance()

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &instance)
	c.Assert(err, check.IsNil)

	params := BindAppParameters(map[string]interface{}{
		"param1": "val1",
	})
	envs, err := client.BindApp(context.TODO(), &instance, a, params, ev, "request-id")
	c.Assert(err, check.IsNil)
	c.Assert(envs, check.DeepEquals, map[string]string{
		"env1": "val1",
		"env2": "val2",
		"env3": "3",
	})
	storedInstance, err := GetServiceInstance(context.TODO(), instance.ServiceName, instance.Name)
	c.Assert(err, check.IsNil)
	c.Assert(storedInstance.BrokerData, check.DeepEquals, &BrokerInstanceData{
		UUID:             "e7252f14-54be-45df-bd40-e988a0e41059",
		ServiceID:        "s1",
		PlanID:           "p1",
		LastOperationKey: "Binding",
		Binds: map[string]BrokerInstanceBind{
			"theapp": {
				UUID:         bindID,
				OperationKey: "Binding",
				Parameters:   params,
			},
		},
	})
}

func (s *S) TestBrokerClientUnbindApp(c *check.C) {
	ev := createEvt(c)
	reaction := func(req *osb.UnbindRequest) (*osb.UnbindResponse, error) {
		exID, err := json.Marshal(map[string]interface{}{
			"user": "my@user",
		})
		c.Assert(err, check.IsNil)
		c.Assert(req, check.DeepEquals, &osb.UnbindRequest{
			InstanceID:        "e7252f14-54be-45df-bd40-e988a0e41059",
			ServiceID:         "s1",
			PlanID:            "p1",
			BindingID:         "xxxx-xxxx",
			AcceptsIncomplete: true,
			OriginatingIdentity: &osb.OriginatingIdentity{
				Platform: "tsuru",
				Value:    string(exID),
			},
		})
		opKey := osb.OperationKey("Unbinding")
		return &osb.UnbindResponse{OperationKey: &opKey}, nil
	}
	config := osbfake.FakeClientConfiguration{
		CatalogReaction: &osbfake.CatalogReaction{Response: &osb.CatalogResponse{
			Services: []osb.Service{
				{
					ID:   "s1",
					Name: "service",
					Plans: []osb.Plan{
						{ID: "p1", Name: "plan1", Description: "First plan"},
					},
				},
			},
		}},
		UnbindReaction: osbfake.DynamicUnbindReaction(reaction),
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	client, err := newClient(serviceTypes.Broker{Name: "broker"}, "service")
	c.Assert(err, check.IsNil)
	instance := createTestInstance()
	instance.BrokerData.Binds = map[string]BrokerInstanceBind{
		"theapp": {
			UUID: "xxxx-xxxx",
		},
	}

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &instance)
	c.Assert(err, check.IsNil)

	a := provisiontest.NewFakeApp("theapp", "python", 1)
	err = client.UnbindApp(context.TODO(), &instance, a, ev, "request-id")
	c.Assert(err, check.IsNil)
	storedInstance, err := GetServiceInstance(context.TODO(), instance.ServiceName, instance.Name)
	c.Assert(err, check.IsNil)
	c.Assert(storedInstance.BrokerData, check.DeepEquals, &BrokerInstanceData{
		UUID:             "e7252f14-54be-45df-bd40-e988a0e41059",
		ServiceID:        "s1",
		PlanID:           "p1",
		LastOperationKey: "Unbinding",
		Binds:            map[string]BrokerInstanceBind{},
	})
}

func (s *S) TestBrokerClientUpdate(c *check.C) {
	ev := createEvt(c)
	planID := "planid"
	reaction := func(req *osb.UpdateInstanceRequest) (*osb.UpdateInstanceResponse, error) {
		exID, err := json.Marshal(map[string]interface{}{
			"user": "my@user",
		})
		c.Assert(err, check.IsNil)
		c.Assert(req, check.DeepEquals, &osb.UpdateInstanceRequest{
			InstanceID: "e7252f14-54be-45df-bd40-e988a0e41059",
			ServiceID:  "serviceid",
			PlanID:     &planID,
			Parameters: map[string]interface{}{
				"param1": "val1",
			},
			OriginatingIdentity: &osb.OriginatingIdentity{
				Platform: "tsuru",
				Value:    string(exID),
			},
			Context: map[string]interface{}{
				"request_id": "request-id",
				"event_id":   ev.UniqueID.Hex(),
			},
			PreviousValues: &osb.PreviousValues{
				ServiceID: "s1",
				PlanID:    "p1",
			},
			AcceptsIncomplete: true,
		})
		opKey := osb.OperationKey("Provisioning")
		return &osb.UpdateInstanceResponse{OperationKey: &opKey}, nil
	}
	config := osbfake.FakeClientConfiguration{
		UpdateInstanceReaction: osbfake.DynamicUpdateInstanceReaction(reaction),
		CatalogReaction: &osbfake.CatalogReaction{
			Response: &osb.CatalogResponse{
				Services: []osb.Service{
					{Name: "service", ID: "serviceid", Plans: []osb.Plan{{Name: "plan1", ID: "planid"}}},
				},
			},
		},
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	client, err := newClient(serviceTypes.Broker{Name: "broker"}, "service")
	c.Assert(err, check.IsNil)
	instance := createTestInstance()

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &instance)
	c.Assert(err, check.IsNil)

	instance.Parameters = map[string]interface{}{
		"param1": "val1",
	}
	err = client.Update(context.TODO(), &instance, ev, "request-id")
	c.Assert(err, check.IsNil)
	storedInstance, err := GetServiceInstance(context.TODO(), instance.ServiceName, instance.Name)
	c.Assert(err, check.IsNil)
	c.Assert(storedInstance.BrokerData, check.DeepEquals, &BrokerInstanceData{
		UUID:             "e7252f14-54be-45df-bd40-e988a0e41059",
		ServiceID:        "serviceid",
		PlanID:           "planid",
		LastOperationKey: "Provisioning",
		Binds:            map[string]BrokerInstanceBind{},
	})
}

func (s *S) TestBrokerClientInfo(c *check.C) {
	client, err := newClient(serviceTypes.Broker{Name: "broker"}, "service")
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{
		Name:      "my-instance",
		PlanName:  "standard",
		TeamOwner: "teamowner",
		Parameters: map[string]interface{}{
			"param1": "val1",
			"param2": 4,
		},
	}
	info, err := client.Info(context.TODO(), &instance, "")
	c.Assert(err, check.IsNil)
	sort.Slice(info, func(i int, j int) bool {
		return info[i]["label"] < info[j]["label"]
	})
	c.Assert(info, check.DeepEquals, []map[string]string{
		{"label": "param1", "value": "val1"},
		{"label": "param2", "value": "4"},
	})
}

func createTestInstance() ServiceInstance {
	return ServiceInstance{
		Name:        "instance",
		ServiceName: "service",
		PlanName:    "plan1",
		TeamOwner:   "teamOwner",
		BrokerData: &BrokerInstanceData{
			UUID:      "e7252f14-54be-45df-bd40-e988a0e41059",
			ServiceID: "s1",
			PlanID:    "p1",
		},
	}
}
