// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

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
	config.Insecure = b.Config.Insecure
	var authConfig *osb.AuthConfig
	if b.Config.AuthConfig != nil {
		authConfig = &osb.AuthConfig{}
		if b.Config.AuthConfig.BasicAuthConfig != nil {
			authConfig.BasicAuthConfig = &osb.BasicAuthConfig{
				Username: b.Config.AuthConfig.BasicAuthConfig.Username,
				Password: b.Config.AuthConfig.BasicAuthConfig.Password,
			}
		}
		if b.Config.AuthConfig.BearerConfig != nil {
			authConfig.BearerConfig = &osb.BearerConfig{
				Token: b.Config.AuthConfig.BearerConfig.Token,
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
	plan, err := getPlan(s, instance.PlanName)
	if err != nil {
		return err
	}
	id, err := idForEvent(evt)
	if err != nil {
		return err
	}
	req := osb.ProvisionRequest{
		InstanceID:          instance.Name,
		ServiceID:           s.ID,
		PlanID:              plan.ID,
		OrganizationGUID:    instance.TeamOwner,
		SpaceGUID:           instance.TeamOwner,
		Parameters:          instance.Parameters,
		OriginatingIdentity: id,
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
	//TODO: consider storing OperationKey for Status call
	return err
}

func (b *brokerClient) Update(instance *ServiceInstance, evt *event.Event, requestID string) error {
	_, s, err := b.getService(b.service)
	if err != nil {
		return err
	}
	plan, err := getPlan(s, instance.PlanName)
	if err != nil {
		return err
	}
	id, err := idForEvent(evt)
	if err != nil {
		return err
	}
	req := osb.UpdateInstanceRequest{
		InstanceID:          instance.Name,
		ServiceID:           s.ID,
		PlanID:              &plan.ID,
		Parameters:          instance.Parameters,
		OriginatingIdentity: id,
		Context: map[string]interface{}{
			"request_id": requestID,
			"event_id":   evt.UniqueID.Hex(),
		},
	}
	_, err = b.client.UpdateInstance(&req)
	if osb.IsAsyncRequiredError(err) {
		// We only set AcceptsIncomplete when it is required because some Brokers fail when
		// they don't support async operations and AcceptsIncomplete is true.
		req.AcceptsIncomplete = true
		_, err = b.client.UpdateInstance(&req)
	}
	// TODO: consider storing OperationKey
	return err
}

func (b *brokerClient) Destroy(instance *ServiceInstance, evt *event.Event, requestID string) error {
	_, s, err := b.getService(b.service)
	if err != nil {
		return err
	}
	plan, err := getPlan(s, instance.PlanName)
	if err != nil {
		return err
	}
	id, err := idForEvent(evt)
	if err != nil {
		return err
	}
	req := osb.DeprovisionRequest{
		InstanceID:          instance.Name,
		ServiceID:           s.ID,
		PlanID:              plan.ID,
		OriginatingIdentity: id,
	}
	_, err = b.client.DeprovisionInstance(&req)
	if osb.IsAsyncRequiredError(err) {
		// We only set AcceptsIncomplete when it is required because some Brokers fail when
		// they don't support async operations and AcceptsIncomplete is true.
		req.AcceptsIncomplete = true
		_, err = b.client.DeprovisionInstance(&req)
	}
	//TODO: consider storing OperatioKey and track async operations
	return err
}

func (b *brokerClient) BindApp(instance *ServiceInstance, app bind.App, params BindAppParameters, evt *event.Event, requestID string) (map[string]string, error) {
	_, s, err := b.getService(b.service)
	if err != nil {
		return nil, err
	}
	plan, err := getPlan(s, instance.PlanName)
	if err != nil {
		return nil, err
	}
	id, err := idForEvent(evt)
	if err != nil {
		return nil, err
	}
	appGUID, err := app.GetUUID()
	if err != nil {
		return nil, err
	}
	req := osb.BindRequest{
		ServiceID:           s.ID,
		InstanceID:          instance.Name,
		PlanID:              plan.ID,
		BindingID:           getBindingID(instance, app),
		AppGUID:             &appGUID,
		Parameters:          params,
		OriginatingIdentity: id,
		BindResource: &osb.BindResource{
			AppGUID: &appGUID,
		},
		Context: map[string]interface{}{
			"request_id": requestID,
			"event_id":   evt.UniqueID.Hex(),
		},
		AcceptsIncomplete: true,
	}
	resp, err := b.client.Bind(&req)
	if osb.IsAsyncBindingOperationsNotAllowedError(err) {
		req.AcceptsIncomplete = false
		resp, err = b.client.Bind(&req)
	}
	if resp == nil {
		return nil, err
	}
	// TODO: consider storing OperationKey
	envs := make(map[string]string)
	for k, v := range resp.Credentials {
		switch s := v.(type) {
		case string:
			envs[k] = s
		case int:
			envs[k] = strconv.Itoa(s)
		}
	}
	return envs, err
}

func (b *brokerClient) UnbindApp(instance *ServiceInstance, app bind.App, evt *event.Event, requestID string) error {
	_, s, err := b.getService(b.service)
	if err != nil {
		return err
	}
	plan, err := getPlan(s, instance.PlanName)
	if err != nil {
		return err
	}
	id, err := idForEvent(evt)
	if err != nil {
		return err
	}
	req := osb.UnbindRequest{
		InstanceID:          instance.Name,
		BindingID:           getBindingID(instance, app),
		ServiceID:           s.ID,
		PlanID:              plan.ID,
		OriginatingIdentity: id,
		AcceptsIncomplete:   true,
	}
	_, err = b.client.Unbind(&req)
	if osb.IsAsyncBindingOperationsNotAllowedError(err) {
		req.AcceptsIncomplete = false
		_, err = b.client.Unbind(&req)
	}
	// TODO: consider storing OperationKey
	return err
}

func (b *brokerClient) Status(instance *ServiceInstance, requestID string) (string, error) {
	_, s, err := b.getService(b.service)
	if err != nil {
		return "", err
	}
	plan, err := getPlan(s, instance.PlanName)
	if err != nil {
		return "", err
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
		PlanID:     &plan.ID,
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
	var params []map[string]string
	for k, v := range instance.Parameters {
		params = append(params, map[string]string{
			"label": k,
			"value": fmt.Sprintf("%v", v),
		})
	}
	return params, nil
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
			Schemas:     p.Schemas,
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

func getPlan(s osb.Service, planName string) (osb.Plan, error) {
	for _, p := range s.Plans {
		if p.Name == planName {
			return p, nil
		}
	}
	return osb.Plan{}, fmt.Errorf("invalid plan: %s", planName)
}

func newService(broker serviceTypes.Broker, osbservice osb.Service) Service {
	return Service{
		Name: fmt.Sprintf("%s%s%s", broker.Name, serviceNameBrokerSep, osbservice.Name),
		Doc:  osbservice.Description,
	}
}

func getBindingID(instance *ServiceInstance, app bind.App) string {
	return fmt.Sprintf("%s-%s", instance.Name, app.GetName())
}

func idForEvent(evt *event.Event) (*osb.OriginatingIdentity, error) {
	identity, err := json.Marshal(map[string]interface{}{
		"user": evt.Owner.Name,
	})
	if err != nil {
		return nil, err
	}
	return &osb.OriginatingIdentity{
		Platform: "tsuru",
		Value:    string(identity),
	}, nil
}
