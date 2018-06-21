// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"fmt"
	"strings"

	"github.com/tsuru/tsuru/storage"
	serviceTypes "github.com/tsuru/tsuru/types/service"
)

type brokerService struct {
	storage serviceTypes.ServiceBrokerStorage
}

func BrokerService() (serviceTypes.ServiceBrokerService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &brokerService{storage: dbDriver.ServiceBrokerStorage}, nil
}

func (b *brokerService) Create(broker serviceTypes.Broker) error {
	return b.storage.Insert(broker)
}

func (b *brokerService) Update(name string, broker serviceTypes.Broker) error {
	return b.storage.Update(name, broker)
}

func (b *brokerService) Delete(name string) error {
	return b.storage.Delete(name)
}

func (b *brokerService) Find(name string) (serviceTypes.Broker, error) {
	return b.storage.Find(name)
}

func (b *brokerService) List() ([]serviceTypes.Broker, error) {
	return b.storage.FindAll()
}

func getBrokeredServices() ([]Service, error) {
	brokerService, err := BrokerService()
	if err != nil {
		return nil, err
	}
	brokers, err := brokerService.List()
	if err != nil {
		return nil, err
	}
	var services []Service
	for _, b := range brokers {
		c, err := newClient(b)
		if err != nil {
			continue
		}
		cat, err := c.client.GetCatalog()
		if err != nil {
			continue
		}
		for _, s := range cat.Services {
			services = append(services, newService(b, s))
		}
	}
	return services, nil
}

// GetBrokeredService retrieves the service information from a service that is
// offered by a broker. name is in the format "<broker>serviceNameBrokerSep<service>".
func getBrokeredService(name string) (Service, error) {
	parts := strings.SplitN(name, serviceNameBrokerSep, 2)
	if len(parts) != 2 {
		return Service{}, fmt.Errorf("name is not in the format <broker>%s<service>", serviceNameBrokerSep)
	}
	brokerName, serviceName := parts[0], parts[1]
	brokerService, err := BrokerService()
	if err != nil {
		return Service{}, err
	}
	broker, err := brokerService.Find(brokerName)
	if err != nil {
		return Service{}, err
	}
	client, err := newClient(broker)
	if err != nil {
		return Service{}, err
	}
	return client.GetService(serviceName)
}

func isBrokeredService(name string) bool {
	return strings.Contains(name, serviceNameBrokerSep)
}
