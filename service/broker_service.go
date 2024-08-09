// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/servicemanager"
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

func (b *brokerService) Create(ctx context.Context, broker serviceTypes.Broker) error {
	return b.storage.Insert(ctx, broker)
}

func (b *brokerService) Update(ctx context.Context, name string, broker serviceTypes.Broker) error {
	if broker.Config.CacheExpirationSeconds == 0 {
		sb, err := b.Find(ctx, name)
		if err != nil {
			return err
		}
		broker.Config.CacheExpirationSeconds = sb.Config.CacheExpirationSeconds
	} else if broker.Config.CacheExpirationSeconds < 0 {
		broker.Config.CacheExpirationSeconds = 0
	}
	return b.storage.Update(ctx, name, broker)
}

func (b *brokerService) Delete(ctx context.Context, name string) error {
	return b.storage.Delete(ctx, name)
}

func (b *brokerService) Find(ctx context.Context, name string) (serviceTypes.Broker, error) {
	return b.storage.Find(ctx, name)
}

func (b *brokerService) List(ctx context.Context) ([]serviceTypes.Broker, error) {
	return b.storage.FindAll(ctx)
}

func getBrokeredServices(ctx context.Context) ([]Service, error) {
	brokerService, err := BrokerService()
	if err != nil {
		return nil, err
	}
	brokers, err := brokerService.List(ctx)
	if err != nil {
		return nil, err
	}
	var services []Service
	for _, b := range brokers {
		c, err := newClient(b, "")
		if err != nil {
			log.Errorf("[Broker=%v] error creating broker client: %v.", b.Name, err)
			continue
		}
		cat, err := c.getCatalog(ctx, b.Name)
		if err != nil {
			log.Errorf("[Broker=%v] error getting catalog: %v.", b.Name, err)
			continue
		}
		for _, s := range cat.Services {
			services = append(services, newService(b, s))
		}
	}
	return services, nil
}

// getBrokeredService retrieves the service information from a service that is
// offered by a broker. name is in the format "<broker>serviceNameBrokerSep<service>".
func getBrokeredService(ctx context.Context, name string) (Service, error) {
	catalogName, serviceName, err := splitBrokerService(name)
	if err != nil {
		return Service{}, err
	}
	client, err := newBrokeredServiceClient(name)
	if err != nil {
		return Service{}, err
	}
	s, _, err := client.getService(ctx, serviceName, catalogName)
	return s, err
}

func isBrokeredService(name string) bool {
	return strings.Contains(name, serviceNameBrokerSep)
}

func splitBrokerService(name string) (string, string, error) {
	parts := strings.SplitN(name, serviceNameBrokerSep, 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("name is not in the format <broker>%s<service>", serviceNameBrokerSep)
	}
	return parts[0], parts[1], nil
}

func newBrokeredServiceClient(service string) (*brokerClient, error) {
	brokerName, serviceName, err := splitBrokerService(service)
	if err != nil {
		return nil, err
	}
	broker, err := servicemanager.ServiceBroker.Find(context.TODO(), brokerName)
	if err != nil {
		return nil, err
	}
	return newClient(broker, serviceName)
}
