package service

import (
	"encoding/json"

	osb "github.com/pmorie/go-open-service-broker-client/v2"
	osbfake "github.com/pmorie/go-open-service-broker-client/v2/fake"
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
	plans, err := client.Plans("")
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
		c.Assert(req, check.DeepEquals, &osb.ProvisionRequest{
			PlanID:           "planid",
			ServiceID:        "serviceid",
			InstanceID:       "my-instance",
			OrganizationGUID: "teamowner",
			SpaceGUID:        "teamowner",
			OriginatingIdentity: &osb.OriginatingIdentity{
				Platform: "tsuru",
				Value:    string(exID),
			},
			Context: map[string]interface{}{
				"request_id":        "request-id",
				"event_id":          ev.UniqueID.Hex(),
				"organization_guid": "teamowner",
				"space_guid":        "teamowner",
			},
		})
		return nil, nil
	}
	config := osbfake.FakeClientConfiguration{
		ProvisionReaction: osbfake.DynamicProvisionReaction(reaction),
		CatalogReaction: &osbfake.CatalogReaction{
			Response: &osb.CatalogResponse{
				Services: []osb.Service{
					{Name: "service", ID: "serviceid", Plans: []osb.Plan{{Name: "standard", ID: "planid"}}},
				},
			},
		},
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	client, err := newClient(serviceTypes.Broker{Name: "broker"}, "service")
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{
		Name:      "my-instance",
		PlanName:  "standard",
		TeamOwner: "teamowner",
	}
	err = client.Create(&instance, ev, "request-id")
	c.Assert(err, check.IsNil)
	c.Assert(provisioned, check.DeepEquals, true)
	provisioned = false
	instance.PlanName = "premium"
	err = client.Create(&instance, ev, "request-id")
	c.Assert(err, check.ErrorMatches, `invalid plan: premium`)
	c.Assert(provisioned, check.DeepEquals, false)
}

func (s *S) TestBrokerClientCreateAsyncRequired(c *check.C) {
	var calls int
	ev := createEvt(c)
	reaction := func(req *osb.ProvisionRequest) (*osb.ProvisionResponse, error) {
		calls++
		if calls > 1 {
			c.Assert(req.AcceptsIncomplete, check.DeepEquals, true)
			return nil, nil
		}
		c.Assert(req.AcceptsIncomplete, check.DeepEquals, false)
		return nil, osbfake.AsyncRequiredError()
	}
	config := osbfake.FakeClientConfiguration{
		ProvisionReaction: osbfake.DynamicProvisionReaction(reaction),
		CatalogReaction: &osbfake.CatalogReaction{
			Response: &osb.CatalogResponse{
				Services: []osb.Service{
					{Name: "service", ID: "serviceid", Plans: []osb.Plan{{Name: "standard", ID: "planid"}}},
				},
			},
		},
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	client, err := newClient(serviceTypes.Broker{Name: "broker"}, "service")
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{
		Name:      "my-instance",
		PlanName:  "standard",
		TeamOwner: "teamowner",
	}
	err = client.Create(&instance, ev, "request-id")
	c.Assert(err, check.IsNil)
	c.Assert(calls, check.DeepEquals, 2)
}

func (s *S) TestBrokerClientStatus(c *check.C) {
	reaction := func(req *osb.LastOperationRequest) (*osb.LastOperationResponse, error) {
		pID := "p2"
		sID := "s1"
		exID, err := json.Marshal(map[string]interface{}{
			"team": "teamOwner",
		})
		c.Assert(err, check.IsNil)
		c.Assert(req, check.DeepEquals, &osb.LastOperationRequest{
			InstanceID: "instance",
			ServiceID:  &sID,
			PlanID:     &pID,
			OriginatingIdentity: &osb.OriginatingIdentity{
				Platform: "tsuru",
				Value:    string(exID),
			},
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
	instance := ServiceInstance{
		Name:        "instance",
		ServiceName: "service",
		PlanName:    "plan2",
		TeamOwner:   "teamOwner",
	}
	status, err := client.Status(&instance, "req-id")
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
			AcceptsIncomplete: true,
			InstanceID:        "instance",
			ServiceID:         "s1",
			PlanID:            "p1",
			OriginatingIdentity: &osb.OriginatingIdentity{
				Platform: "tsuru",
				Value:    string(exID),
			},
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
	instance := ServiceInstance{
		Name:        "instance",
		ServiceName: "service",
		PlanName:    "plan1",
		TeamOwner:   "teamOwner",
	}
	err = client.Destroy(&instance, ev, "req-id")
	c.Assert(err, check.IsNil)
	c.Assert(calls, check.DeepEquals, 1)
}
