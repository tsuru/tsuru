// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package v2

import (
	osb "github.com/pmorie/go-open-service-broker-client/v2"
	serviceTypes "github.com/tsuru/tsuru/types/service"
)

func GetServices() (map[serviceTypes.Broker][]osb.Service, error) {
	brokerService, err := BrokerService()
	if err != nil {
		return nil, err
	}
	brokers, err := brokerService.List()
	if err != nil {
		return nil, err
	}
	catalog := make(map[serviceTypes.Broker][]osb.Service)
	for _, b := range brokers {
		services, err := getServices(b)
		if err != nil {
			//log
		}
		catalog[b] = services
	}
	return catalog, nil
}

func getServices(b serviceTypes.Broker) ([]osb.Service, error) {
	config := osb.DefaultClientConfiguration()
	config.URL = b.URL
	config.AuthConfig = b.AuthConfig
	client, err := osb.NewClient(config)
	if err != nil {
		return nil, err
	}
	cat, err := client.GetCatalog()
	if err != nil {
		return nil, err
	}
	return cat.Services, nil
}
