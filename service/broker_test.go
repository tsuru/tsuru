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
		c.Assert(req.PlanID, check.DeepEquals, "planid")
		c.Assert(req.ServiceID, check.DeepEquals, "serviceid")
		c.Assert(req.InstanceID, check.DeepEquals, "my-instance")
		c.Assert(req.OrganizationGUID, check.DeepEquals, "teamowner")
		c.Assert(req.SpaceGUID, check.DeepEquals, "teamowner")
		exID, err := json.Marshal(map[string]interface{}{
			"user": "my@user",
		})
		c.Assert(err, check.IsNil)
		c.Assert(req.OriginatingIdentity, check.DeepEquals, &osb.OriginatingIdentity{
			Platform: "tsuru",
			Value:    string(exID),
		})
		c.Assert(req.Context, check.DeepEquals, map[string]interface{}{
			"request_id":        "request-id",
			"event_id":          ev.UniqueID.Hex(),
			"organization_guid": "teamowner",
			"space_guid":        "teamowner",
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
