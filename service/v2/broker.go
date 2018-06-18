// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package v2

import (
	osb "github.com/pmorie/go-open-service-broker-client/v2"
	serviceTypes "github.com/tsuru/tsuru/types/service"
)

// ClientFactory provides a way to customize the Open Service
// Broker API client. Should be used in tests to create a fake client.
var ClientFactory = func(config *osb.ClientConfiguration) (osb.Client, error) {
	return osb.NewClient(config)
}

// ServiceBrokerAPI defines the Open Service Broker API contract
type ServiceBrokerAPI interface {
	osb.Client
}

// BrokerClient implements the Open Service Broker API for stored
// Brokers
type BrokerClient struct {
	broker serviceTypes.Broker
	client osb.Client
}

// NewClient configures a client that provides a Service Broker API
// implementation
func NewClient(b serviceTypes.Broker) (ServiceBrokerAPI, error) {
	broker := BrokerClient{broker: b}
	config := osb.DefaultClientConfiguration()
	config.URL = b.URL
	var authConfig *osb.AuthConfig
	if b.AuthConfig != nil {
		authConfig = &osb.AuthConfig{}
		if b.AuthConfig.BasicAuthConfig != nil {
			authConfig.BasicAuthConfig = &osb.BasicAuthConfig{
				Username: b.AuthConfig.BasicAuthConfig.Username,
				Password: b.AuthConfig.BasicAuthConfig.Password,
			}
		}
		if b.AuthConfig.BearerConfig != nil {
			authConfig.BearerConfig = &osb.BearerConfig{
				Token: b.AuthConfig.BearerConfig.Token,
			}
		}
	}
	config.AuthConfig = authConfig
	client, err := ClientFactory(config)
	if err != nil {
		return nil, err
	}
	broker.client = client
	return &broker, nil
}

func (b *BrokerClient) GetCatalog() (*osb.CatalogResponse, error) {
	return b.client.GetCatalog()
}

func (b *BrokerClient) ProvisionInstance(r *osb.ProvisionRequest) (*osb.ProvisionResponse, error) {
	return b.client.ProvisionInstance(r)
}

func (b *BrokerClient) DeprovisionInstance(r *osb.DeprovisionRequest) (*osb.DeprovisionResponse, error) {
	return b.client.DeprovisionInstance(r)
}

func (b *BrokerClient) UpdateInstance(r *osb.UpdateInstanceRequest) (*osb.UpdateInstanceResponse, error) {
	return b.client.UpdateInstance(r)
}

func (b *BrokerClient) PollLastOperation(r *osb.LastOperationRequest) (*osb.LastOperationResponse, error) {
	return b.client.PollLastOperation(r)
}

func (b *BrokerClient) GetBinding(r *osb.GetBindingRequest) (*osb.GetBindingResponse, error) {
	return b.client.GetBinding(r)
}

func (b *BrokerClient) Unbind(r *osb.UnbindRequest) (*osb.UnbindResponse, error) {
	return b.client.Unbind(r)
}

func (b *BrokerClient) Bind(r *osb.BindRequest) (*osb.BindResponse, error) {
	return b.client.Bind(r)
}

func (b *BrokerClient) PollBindingLastOperation(r *osb.BindingLastOperationRequest) (*osb.LastOperationResponse, error) {
	return b.client.PollBindingLastOperation(r)
}
