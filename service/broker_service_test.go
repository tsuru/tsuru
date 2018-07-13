package service

import (
	"testing"

	"github.com/tsuru/config"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"github.com/tsuru/tsuru/types/service"
	check "gopkg.in/check.v1"
)

type BrokerSuite struct {
	service *brokerService
}

var _ = check.Suite(&BrokerSuite{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *BrokerSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_service_v2_tests")
	svc, err := BrokerService()
	c.Assert(err, check.IsNil)
	s.service = svc.(*brokerService)
}

func (s *BrokerSuite) SetUpTest(c *check.C) {
	brokers, err := s.service.List()
	c.Assert(err, check.IsNil)
	for _, b := range brokers {
		errDel := s.service.Delete(b.Name)
		c.Assert(errDel, check.IsNil)
	}
}

func (s *BrokerSuite) TestServiceBrokerCreate(c *check.C) {
	err := s.service.Create(service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
	})
	c.Assert(err, check.IsNil)
	broker, err := s.service.Find("broker-name")
	c.Assert(err, check.IsNil)
	c.Assert(broker.URL, check.DeepEquals, "https://localhost:8080")
}

func (s *BrokerSuite) TestServiceBrokerUpdate(c *check.C) {
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

func (s *BrokerSuite) TestServiceBrokerDelete(c *check.C) {
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

func (s *BrokerSuite) TestServiceBrokerFind(c *check.C) {
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

func (s *BrokerSuite) TestServiceBrokerList(c *check.C) {
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
		{
			Name: "broker-name",
			URL:  "https://localhost:8080",
			Config: service.BrokerConfig{
				Context: map[string]interface{}{},
			},
		},
		{
			Name: "broker-2",
			URL:  "https://localhost:9090",
			Config: service.BrokerConfig{
				Context: map[string]interface{}{},
			},
		},
	})
}
