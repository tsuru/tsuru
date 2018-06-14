package v2

import (
	"testing"

	osb "github.com/pmorie/go-open-service-broker-client/v2"
	fake "github.com/pmorie/go-open-service-broker-client/v2/fake"
	"github.com/tsuru/config"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"github.com/tsuru/tsuru/types/service"
	check "gopkg.in/check.v1"
)

type S struct {
	service *brokerService
}

var _ = check.Suite(&S{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_service_v2_tests")
	svc, err := BrokerService()
	c.Assert(err, check.IsNil)
	s.service = svc.(*brokerService)
}

func (s *S) SetUpTest(c *check.C) {
	brokers, err := s.service.List()
	c.Assert(err, check.IsNil)
	for _, b := range brokers {
		errDel := s.service.Delete(b.Name)
		c.Assert(errDel, check.IsNil)
	}
}

func (s *S) TestServiceBrokerCreate(c *check.C) {
	err := s.service.Create(service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
	})
	c.Assert(err, check.IsNil)
	broker, err := s.service.Find("broker-name")
	c.Assert(err, check.IsNil)
	c.Assert(broker.URL, check.DeepEquals, "https://localhost:8080")
}

func (s *S) TestServiceBrokerUpdate(c *check.C) {
	err := s.service.Create(service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
	})
	c.Assert(err, check.IsNil)
	err = s.service.Update("broker-name", service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:9090",
	})
	c.Assert(err, check.IsNil)
	broker, err := s.service.Find("broker-name")
	c.Assert(err, check.IsNil)
	c.Assert(broker.URL, check.DeepEquals, "https://localhost:9090")
}

func (s *S) TestServiceBrokerDelete(c *check.C) {
	err := s.service.Create(service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
	})
	c.Assert(err, check.IsNil)
	err = s.service.Delete("broker-name")
	c.Assert(err, check.IsNil)
	brokers, err := s.service.List()
	c.Assert(err, check.IsNil)
	c.Assert(brokers, check.DeepEquals, []service.Broker(nil))
}

func (s *S) TestServiceBrokerFind(c *check.C) {
	err := s.service.Create(service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
	})
	c.Assert(err, check.IsNil)
	err = s.service.Create(service.Broker{
		Name: "broker-2",
		URL:  "https://localhost:9090",
	})
	c.Assert(err, check.IsNil)
	broker, err := s.service.Find("broker-2")
	c.Assert(err, check.IsNil)
	c.Assert(broker.URL, check.Equals, "https://localhost:9090")
}

func (s *S) TestServiceBrokerList(c *check.C) {
	err := s.service.Create(service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
	})
	c.Assert(err, check.IsNil)
	err = s.service.Create(service.Broker{
		Name: "broker-2",
		URL:  "https://localhost:9090",
	})
	c.Assert(err, check.IsNil)
	brokers, err := s.service.List()
	c.Assert(err, check.IsNil)
	c.Assert(brokers, check.DeepEquals, []service.Broker{
		{Name: "broker-name", URL: "https://localhost:8080"},
		{Name: "broker-2", URL: "https://localhost:9090"},
	})
}

func (s *S) TestServiceBrokerGetCatalog(c *check.C) {
	config := fake.FakeClientConfiguration{
		CatalogReaction: &fake.CatalogReaction{
			Response: catalogResponse(),
		},
	}
	clientFactory = fake.NewFakeClientFunc(config)
	catalog, err := s.service.GetCatalog(service.Broker{Name: "fake-broker"})
	c.Assert(err, check.IsNil)
	c.Assert(catalog.Services, check.DeepEquals, catalogResponse().Services)
}

func catalogResponse() *osb.CatalogResponse {
	return &osb.CatalogResponse{
		Services: []osb.Service{
			{
				ID:          "acb56d7c-XXXX-XXXX-XXXX-feb140a59a66",
				Name:        "fake-service",
				Description: "fake service",
				Tags: []string{
					"tag1",
					"tag2",
				},
				Requires: []string{
					"route_forwarding",
				},
				Plans: []osb.Plan{
					{
						ID:          "d3031751-XXXX-XXXX-XXXX-a42377d3320e",
						Name:        "fake-plan-1",
						Description: "description1",
						Metadata: map[string]interface{}{
							"b": "c",
							"d": "e",
						},
					},
				},
				DashboardClient: &osb.DashboardClient{
					ID:          "398e2f8e-XXXX-XXXX-XXXX-19a71ecbcf64",
					Secret:      "277cabb0-XXXX-XXXX-XXXX-7822c0a90e5d",
					RedirectURI: "http://localhost:1234",
				},
				Metadata: map[string]interface{}{
					"a": "b",
					"c": "d",
				},
			},
		},
	}
}
