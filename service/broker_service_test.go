package service

import (
	"context"

	"github.com/tsuru/config"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"github.com/tsuru/tsuru/types/service"
	check "gopkg.in/check.v1"
)

type BrokerSuite struct {
	service *brokerService
}

var _ = check.Suite(&BrokerSuite{})

func (s *BrokerSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_service_v2_tests")
	svc, err := BrokerService()
	c.Assert(err, check.IsNil)
	s.service = svc.(*brokerService)
}

func (s *BrokerSuite) SetUpTest(c *check.C) {
	brokers, err := s.service.List(context.TODO())
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
	broker, err := s.service.Find(context.TODO(), "broker-name")
	c.Assert(err, check.IsNil)
	c.Assert(broker.URL, check.DeepEquals, "https://localhost:8080")
}

func (s *BrokerSuite) TestServiceBrokerUpdateWithCache(c *check.C) {
	err := s.service.Create(service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
	})
	c.Assert(err, check.IsNil)
	err = s.service.Update(context.TODO(), "broker-name", service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:9090",
		Config: service.BrokerConfig{
			CacheExpirationSeconds: 120,
		},
	})
	c.Assert(err, check.IsNil)
	broker, err := s.service.Find(context.TODO(), "broker-name")
	c.Assert(err, check.IsNil)
	c.Assert(broker.URL, check.DeepEquals, "https://localhost:9090")
	c.Assert(broker.Config.CacheExpirationSeconds, check.Equals, 120)
}

func (s *BrokerSuite) TestServiceBrokerUpdateWithoutCache(c *check.C) {
	err := s.service.Create(service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
		Config: service.BrokerConfig{
			CacheExpirationSeconds: 60,
		},
	})
	c.Assert(err, check.IsNil)
	err = s.service.Update(context.TODO(), "broker-name", service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:9090",
	})
	c.Assert(err, check.IsNil)
	broker, err := s.service.Find(context.TODO(), "broker-name")
	c.Assert(err, check.IsNil)
	c.Assert(broker.URL, check.DeepEquals, "https://localhost:9090")
	c.Assert(broker.Config.CacheExpirationSeconds, check.Equals, 60)
}

func (s *BrokerSuite) TestServiceBrokerUpdateDefaultCache(c *check.C) {
	err := s.service.Create(service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
		Config: service.BrokerConfig{
			CacheExpirationSeconds: 60,
		},
	})
	c.Assert(err, check.IsNil)
	err = s.service.Update(context.TODO(), "broker-name", service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:9090",
		Config: service.BrokerConfig{
			CacheExpirationSeconds: -1,
		},
	})
	c.Assert(err, check.IsNil)
	broker, err := s.service.Find(context.TODO(), "broker-name")
	c.Assert(err, check.IsNil)
	c.Assert(broker.URL, check.DeepEquals, "https://localhost:9090")
	c.Assert(broker.Config.CacheExpirationSeconds, check.Equals, 0)
}

func (s *BrokerSuite) TestServiceBrokerDelete(c *check.C) {
	err := s.service.Create(service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
	})
	c.Assert(err, check.IsNil)
	err = s.service.Delete("broker-name")
	c.Assert(err, check.IsNil)
	brokers, err := s.service.List(context.TODO())
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
	broker, err := s.service.Find(context.TODO(), "broker-2")
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
	brokers, err := s.service.List(context.TODO())
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
