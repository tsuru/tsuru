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

// brokerClient implements the Open Service Broker API for stored
// Brokers
type brokerClient struct {
	broker serviceTypes.Broker
	client osb.Client
}

func newClient(b serviceTypes.Broker) (ServiceClient, error) {
	broker := brokerClient{broker: b}
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

func (b *brokerClient) Create(instance *ServiceInstance, evt *event.Event, requestID string) error {
	return fmt.Errorf("not implemented")
}

func (b *brokerClient) Update(instance *ServiceInstance, evt *event.Event, requestID string) error {
	return fmt.Errorf("not implemented")
}

func (b *brokerClient) Destroy(instance *ServiceInstance, evt *event.Event, requestID string) error {
	return fmt.Errorf("not implemented")
}

func (b *brokerClient) BindApp(instance *ServiceInstance, app bind.App, evt *event.Event, requestID string) (map[string]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *brokerClient) UnbindApp(instance *ServiceInstance, app bind.App, evt *event.Event, requestID string) error {
	return fmt.Errorf("not implemented")
}

func (b *brokerClient) Status(instance *ServiceInstance, requestID string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (b *brokerClient) Info(instance *ServiceInstance, requestID string) ([]map[string]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *brokerClient) Plans(requestID string) ([]Plan, error) {
	return nil, fmt.Errorf("not implemented")
}

// Proxy is not implemented for OSB API implementations
func (b *brokerClient) Proxy(path string, evt *event.Event, requestID string, w http.ResponseWriter, r *http.Request) error {
	return fmt.Errorf("service proxy is not available for broker services")
}

// UnbindUnit is a no-op for OSB API implementations
func (b *brokerClient) UnbindUnit(instance *ServiceInstance, app bind.App, unit bind.Unit) error {
	return nil
}

// UnbindUnit is a no-op for OSB API implementations
func (b *brokerClient) BindUnit(instance *ServiceInstance, app bind.App, unit bind.Unit) error {
	return nil
}
