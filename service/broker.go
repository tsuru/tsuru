// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"encoding/json"
	"fmt"
	"net/http"

	osb "github.com/pmorie/go-open-service-broker-client/v2"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/event"
	serviceTypes "github.com/tsuru/tsuru/types/service"
)

const serviceNameBrokerSep = "::"

// ClientFactory provides a way to customize the Open Service
// Broker API client. Should be used in tests to create a fake client.
var ClientFactory = func(config *osb.ClientConfiguration) (osb.Client, error) {
	return osb.NewClient(config)
}

// brokerClient implements the Open Service Broker API for stored
// Brokers
type brokerClient struct {
	broker  serviceTypes.Broker
	service string
	client  osb.Client
}

var _ ServiceClient = &brokerClient{}

func newClient(b serviceTypes.Broker, service string) (*brokerClient, error) {
	broker := brokerClient{
		broker:  b,
		service: service,
	}
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
	_, s, err := b.getService(b.service)
	if err != nil {
		return err
	}
	var planID string
	for _, p := range s.Plans {
		if p.Name == instance.PlanName {
			planID = p.ID
		}
	}
	if planID == "" {
		return fmt.Errorf("invalid plan: %v", instance.PlanName)
	}
	identity, err := json.Marshal(map[string]interface{}{
		"user": evt.Owner.Name,
	})
	if err != nil {
		return err
	}
	req := osb.ProvisionRequest{
		InstanceID:       instance.Name,
		ServiceID:        s.ID,
		PlanID:           planID,
		OrganizationGUID: instance.TeamOwner,
		SpaceGUID:        instance.TeamOwner,
		Parameters:       instance.Parameters,
		OriginatingIdentity: &osb.OriginatingIdentity{
			Platform: "tsuru",
			Value:    string(identity),
		},
		Context: map[string]interface{}{
			"request_id":        requestID,
			"event_id":          evt.UniqueID.Hex(),
			"organization_guid": instance.TeamOwner,
			"space_guid":        instance.TeamOwner,
		},
	}
	_, err = b.client.ProvisionInstance(&req)
	if osb.IsAsyncRequiredError(err) {
		// We only set AcceptsIncomplete when it is required because some Brokers fail when
		// they don't support async operations and AcceptsIncomplete is true.
		req.AcceptsIncomplete = true
		_, err = b.client.ProvisionInstance(&req)
	}
	//TODO: store OperationKey
	return err
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
	_, s, err := b.getService(b.service)
	if err != nil {
		return "", err
	}
	var planID *string
	for i, p := range s.Plans {
		if p.Name == instance.PlanName {
			planID = &s.Plans[i].ID
		}
	}
	origID, err := json.Marshal(map[string]interface{}{
		"team": instance.TeamOwner,
	})
	if err != nil {
		return "", err
	}
	//TODO: send OperationKey
	op, err := b.client.PollLastOperation(&osb.LastOperationRequest{
		ServiceID:  &s.ID,
		PlanID:     planID,
		InstanceID: instance.Name,
		OriginatingIdentity: &osb.OriginatingIdentity{
			Platform: "tsuru",
			Value:    string(origID),
		},
	})
	if err != nil {
		return "", err
	}
	output := string(op.State)
	if op.Description != nil {
		output += " - " + *op.Description
	}
	return output, nil
}

func (b *brokerClient) Info(instance *ServiceInstance, requestID string) ([]map[string]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *brokerClient) Plans(_ string) ([]Plan, error) {
	_, s, err := b.getService(b.service)
	if err != nil {
		return nil, err
	}
	plans := make([]Plan, len(s.Plans))
	for i, p := range s.Plans {
		plans[i] = Plan{
			Name:        p.Name,
			Description: p.Description,
		}
	}
	return plans, nil
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

// Update  is a no-op for OSB API implementations
func (b *brokerClient) Update(instance *ServiceInstance, evt *event.Event, requestID string) error {
	return nil
}

func (b *brokerClient) getService(name string) (Service, osb.Service, error) {
	cat, err := b.client.GetCatalog()
	if err != nil {
		return Service{}, osb.Service{}, err
	}
	for _, s := range cat.Services {
		if s.Name == name {
			return newService(b.broker, s), s, nil
		}
	}
	return Service{}, osb.Service{}, ErrServiceNotFound
}

func newService(broker serviceTypes.Broker, osbservice osb.Service) Service {
	return Service{
		Name: fmt.Sprintf("%s%s%s", broker.Name, serviceNameBrokerSep, osbservice.Name),
		Doc:  osbservice.Description,
	}
}
