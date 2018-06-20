// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"fmt"
	"net/http"

	osb "github.com/pmorie/go-open-service-broker-client/v2"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/event"
	serviceTypes "github.com/tsuru/tsuru/types/service"
)

// ClientFactory provides a way to customize the Open Service
// Broker API client. Should be used in tests to create a fake client.
var ClientFactory = func(config *osb.ClientConfiguration) (osb.Client, error) {
	return osb.NewClient(config)
}

// BrokerClient implements the Open Service Broker API for stored
// Brokers
type BrokerClient struct {
	broker serviceTypes.Broker
	client osb.Client
}

// NewClient configures a client that provides a Service Broker API
// implementation
func NewClient(b serviceTypes.Broker) (ServiceClient, error) {
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

func (b *BrokerClient) Create(instance *ServiceInstance, evt *event.Event, requestID string) error {
	return fmt.Errorf("not implemented")
}

func (b *BrokerClient) Update(instance *ServiceInstance, evt *event.Event, requestID string) error {
	return fmt.Errorf("not implemented")
}

func (b *BrokerClient) Destroy(instance *ServiceInstance, evt *event.Event, requestID string) error {
	return fmt.Errorf("not implemented")
}

func (b *BrokerClient) BindApp(instance *ServiceInstance, app bind.App, evt *event.Event, requestID string) (map[string]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *BrokerClient) UnbindApp(instance *ServiceInstance, app bind.App, evt *event.Event, requestID string) error {
	return fmt.Errorf("not implemented")
}

func (b *BrokerClient) Status(instance *ServiceInstance, requestID string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (b *BrokerClient) Info(instance *ServiceInstance, requestID string) ([]map[string]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *BrokerClient) Plans(requestID string) ([]Plan, error) {
	return nil, fmt.Errorf("not implemented")
}

// Proxy is not implemented for OSB API implementations
func (b *BrokerClient) Proxy(path string, evt *event.Event, requestID string, w http.ResponseWriter, r *http.Request) error {
	return fmt.Errorf("service proxy is not available for broker services")
}

// UnbindUnit is a no-op for OSB API implementations
func (b *BrokerClient) UnbindUnit(instance *ServiceInstance, app bind.App, unit bind.Unit) error {
	return nil
}

// UnbindUnit is a no-op for OSB API implementations
func (b *BrokerClient) BindUnit(instance *ServiceInstance, app bind.App, unit bind.Unit) error {
	return nil
}
