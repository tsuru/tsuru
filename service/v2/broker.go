// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package v2

import (
	osb "github.com/pmorie/go-open-service-broker-client/v2"
	"github.com/tsuru/tsuru/storage"
	serviceTypes "github.com/tsuru/tsuru/types/service"
)

var _ serviceTypes.ServiceBrokerService = &brokerService{}

type brokerService struct {
	storage serviceTypes.ServiceBrokerStorage
}

var clientFactory = func(config *osb.ClientConfiguration) (osb.Client, error) {
	return osb.NewClient(config)
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

func (b *brokerService) GetCatalog(broker serviceTypes.Broker) (serviceTypes.Catalog, error) {
	client, err := newClient(broker)
	if err != nil {
		return serviceTypes.Catalog{}, err
	}
	cat, err := client.GetCatalog()
	if err != nil {
		return serviceTypes.Catalog{}, err
	}
	return serviceTypes.Catalog{Services: cat.Services}, nil
}

func newClient(broker serviceTypes.Broker) (osb.Client, error) {
	config := osb.DefaultClientConfiguration()
	config.URL = broker.URL
	config.AuthConfig = broker.AuthConfig
	return clientFactory(config)
}
