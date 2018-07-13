// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"github.com/tsuru/tsuru/types/service"
	check "gopkg.in/check.v1"
)

type ServiceBrokerSuite struct {
	SuiteHooks
	ServiceBrokerStorage service.ServiceBrokerStorage
}

func (s *ServiceBrokerSuite) TestInsert(c *check.C) {
	broker := service.Broker{
		Name: "broker",
		URL:  "https://localhost:8080",
		Config: service.BrokerConfig{
			AuthConfig: &service.AuthConfig{
				BasicAuthConfig: &service.BasicAuthConfig{
					Username: "user",
					Password: "password",
				},
			},
			Context: map[string]interface{}{
				"Namespace": "broker-namespace",
			},
		},
	}
	err := s.ServiceBrokerStorage.Insert(broker)
	c.Assert(err, check.IsNil)
	b, err := s.ServiceBrokerStorage.Find("broker")
	c.Assert(err, check.IsNil)
	c.Assert(b, check.DeepEquals, broker)
}

func (s *ServiceBrokerSuite) TestInsertDuplicate(c *check.C) {
	broker := service.Broker{
		Name: "broker",
		URL:  "https://localhost:8080",
		Config: service.BrokerConfig{
			AuthConfig: &service.AuthConfig{
				BasicAuthConfig: &service.BasicAuthConfig{
					Username: "user",
					Password: "password",
				},
			},
		},
	}
	err := s.ServiceBrokerStorage.Insert(broker)
	c.Assert(err, check.IsNil)
	err = s.ServiceBrokerStorage.Insert(broker)
	c.Assert(err, check.Equals, service.ErrServiceBrokerAlreadyExists)
}

func (s *ServiceBrokerSuite) TestUpdate(c *check.C) {
	broker := service.Broker{
		Name: "broker",
		URL:  "https://localhost:8080",
		Config: service.BrokerConfig{
			AuthConfig: &service.AuthConfig{
				BasicAuthConfig: &service.BasicAuthConfig{
					Username: "user",
					Password: "password",
				},
			},
		},
	}
	err := s.ServiceBrokerStorage.Insert(broker)
	c.Assert(err, check.IsNil)
	broker.Config.AuthConfig.BasicAuthConfig.Password = "new-password"
	err = s.ServiceBrokerStorage.Update("broker", broker)
	c.Assert(err, check.IsNil)
	broker, err = s.ServiceBrokerStorage.Find("broker")
	c.Assert(err, check.IsNil)
	c.Assert(broker.Config.AuthConfig.BasicAuthConfig.Password, check.Equals, "new-password")
}

func (s *ServiceBrokerSuite) TestUpdateNotFound(c *check.C) {
	broker := service.Broker{
		Name: "broker",
		URL:  "https://localhost:8080",
		Config: service.BrokerConfig{
			AuthConfig: &service.AuthConfig{
				BasicAuthConfig: &service.BasicAuthConfig{
					Username: "user",
					Password: "password",
				},
			},
		},
	}
	err := s.ServiceBrokerStorage.Update("broker", broker)
	c.Assert(err, check.DeepEquals, service.ErrServiceBrokerNotFound)
}

func (s *ServiceBrokerSuite) TestDelete(c *check.C) {
	broker := service.Broker{
		Name: "broker",
		URL:  "https://localhost:8080",
		Config: service.BrokerConfig{
			AuthConfig: &service.AuthConfig{
				BasicAuthConfig: &service.BasicAuthConfig{
					Username: "user",
					Password: "password",
				},
			},
		},
	}
	err := s.ServiceBrokerStorage.Insert(broker)
	c.Assert(err, check.IsNil)
	err = s.ServiceBrokerStorage.Delete("broker")
	c.Assert(err, check.IsNil)
	_, err = s.ServiceBrokerStorage.Find("broker")
	c.Assert(err, check.DeepEquals, service.ErrServiceBrokerNotFound)
}

func (s *ServiceBrokerSuite) TestDeleteNotFound(c *check.C) {
	err := s.ServiceBrokerStorage.Delete("not-found")
	c.Assert(err, check.DeepEquals, service.ErrServiceBrokerNotFound)
}

func (s *ServiceBrokerSuite) TestFindAll(c *check.C) {
	err := s.ServiceBrokerStorage.Insert(service.Broker{
		Name: "broker",
	})
	c.Assert(err, check.IsNil)
	err = s.ServiceBrokerStorage.Insert(service.Broker{
		Name: "broker-2",
	})
	c.Assert(err, check.IsNil)
	brokers, err := s.ServiceBrokerStorage.FindAll()
	c.Assert(err, check.IsNil)
	c.Assert(len(brokers), check.Equals, 2)
}

func (s *ServiceBrokerSuite) TestFind(c *check.C) {
	broker := service.Broker{
		Name: "broker",
		URL:  "https://localhost:8080",
		Config: service.BrokerConfig{
			AuthConfig: &service.AuthConfig{
				BasicAuthConfig: &service.BasicAuthConfig{
					Username: "user",
					Password: "password",
				},
			},
			Context: map[string]interface{}{
				"Namespace": "broker-namespace",
			},
		},
	}
	err := s.ServiceBrokerStorage.Insert(broker)
	c.Assert(err, check.IsNil)
	b, err := s.ServiceBrokerStorage.Find("broker")
	c.Assert(err, check.IsNil)
	c.Assert(b, check.DeepEquals, broker)
}

func (s *ServiceBrokerSuite) TestFindNotFound(c *check.C) {
	broker, err := s.ServiceBrokerStorage.Find("not-found")
	c.Assert(err, check.DeepEquals, service.ErrServiceBrokerNotFound)
	c.Assert(broker, check.DeepEquals, service.Broker{})
}
