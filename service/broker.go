// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	uuid "github.com/nu7hatch/gouuid"
	"github.com/pkg/errors"
	osb "github.com/pmorie/go-open-service-broker-client/v2"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/servicemanager"
	jobTypes "github.com/tsuru/tsuru/types/job"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

var ErrInvalidBrokerData = errors.New("Invalid broker data")

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

func convertResponseToCatalog(response osb.CatalogResponse) serviceTypes.BrokerCatalog {
	cat := serviceTypes.BrokerCatalog{
		Services: make([]serviceTypes.BrokerService, len(response.Services)),
	}
	for i, s := range response.Services {
		cat.Services[i].ID = s.ID
		cat.Services[i].Name = s.Name
		cat.Services[i].Description = s.Description
		cat.Services[i].Plans = make([]serviceTypes.BrokerPlan, len(s.Plans))
		for j, p := range s.Plans {
			cat.Services[i].Plans[j].ID = p.ID
			cat.Services[i].Plans[j].Name = p.Name
			cat.Services[i].Plans[j].Description = p.Description
			if p.Schemas != nil {
				cat.Services[i].Plans[j].Schemas = *p.Schemas
			}
		}
	}
	return cat
}

func convertCatalogToResponse(catalog serviceTypes.BrokerCatalog) osb.CatalogResponse {
	cat := osb.CatalogResponse{
		Services: make([]osb.Service, len(catalog.Services)),
	}
	for i, s := range catalog.Services {
		cat.Services[i].ID = s.ID
		cat.Services[i].Name = s.Name
		cat.Services[i].Description = s.Description
		cat.Services[i].Plans = make([]osb.Plan, len(s.Plans))
		for j, p := range s.Plans {
			cat.Services[i].Plans[j].ID = p.ID
			cat.Services[i].Plans[j].Name = p.Name
			cat.Services[i].Plans[j].Description = p.Description
			if schemas, ok := p.Schemas.(osb.Schemas); ok {
				cat.Services[i].Plans[j].Schemas = &schemas
			}
		}
	}
	return cat
}

func (b *brokerClient) Create(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error {
	_, s, err := b.getService(ctx, b.service, instance.Name)
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
	uid, err := uuid.NewV4()
	if err != nil {
		return errors.WithMessage(err, "failed to generate instance uuid")
	}
	orgID, err := uuid.NewV4()
	if err != nil {
		return errors.WithMessage(err, "failed to generate org/space uuid")
	}
	instance.BrokerData = &BrokerInstanceData{
		UUID:      uid.String(),
		ServiceID: s.ID,
		PlanID:    plan.ID,
		OrgID:     orgID.String(),
		SpaceID:   orgID.String(),
	}
	req := osb.ProvisionRequest{
		InstanceID:          instance.BrokerData.UUID,
		ServiceID:           instance.BrokerData.ServiceID,
		PlanID:              instance.BrokerData.PlanID,
		OrganizationGUID:    instance.BrokerData.OrgID,
		SpaceGUID:           instance.BrokerData.SpaceID,
		Parameters:          instance.Parameters,
		OriginatingIdentity: id,
		Context: map[string]interface{}{
			"request_id":        requestID,
			"event_id":          evt.UniqueID.Hex(),
			"organization_guid": instance.BrokerData.OrgID,
			"space_guid":        instance.BrokerData.SpaceID,
		},
		AcceptsIncomplete: true,
	}
	for k, v := range b.broker.Config.Context {
		req.Context[k] = v
	}
	resp, err := b.client.ProvisionInstance(&req)
	if err != nil {
		return err
	}
	if resp != nil && resp.OperationKey != nil {
		instance.BrokerData.LastOperationKey = string(*resp.OperationKey)
	}
	return nil
}

func (b *brokerClient) Update(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error {
	if instance.BrokerData == nil {
		return ErrInvalidBrokerData
	}
	_, s, err := b.getService(ctx, b.service, instance.Name)
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
		InstanceID:          instance.BrokerData.UUID,
		ServiceID:           s.ID,
		PlanID:              &plan.ID,
		Parameters:          instance.Parameters,
		OriginatingIdentity: id,
		Context: map[string]interface{}{
			"request_id": requestID,
			"event_id":   evt.UniqueID.Hex(),
		},
		PreviousValues: &osb.PreviousValues{
			PlanID:    instance.BrokerData.PlanID,
			ServiceID: instance.BrokerData.ServiceID,
			OrgID:     instance.BrokerData.OrgID,
			SpaceID:   instance.BrokerData.SpaceID,
		},
		AcceptsIncomplete: true,
	}
	for k, v := range b.broker.Config.Context {
		req.Context[k] = v
	}
	instance.BrokerData.PlanID = plan.ID
	instance.BrokerData.ServiceID = s.ID
	resp, err := b.client.UpdateInstance(&req)
	if err != nil {
		return err
	}
	if resp != nil && resp.OperationKey != nil {
		instance.BrokerData.LastOperationKey = string(*resp.OperationKey)
	}
	return updateBrokerData(ctx, instance)
}

func (b *brokerClient) Destroy(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error {
	if instance.BrokerData == nil {
		return nil
	}
	id, err := idForEvent(evt)
	if err != nil {
		return err
	}
	req := osb.DeprovisionRequest{
		InstanceID:          instance.BrokerData.UUID,
		ServiceID:           instance.BrokerData.ServiceID,
		PlanID:              instance.BrokerData.PlanID,
		OriginatingIdentity: id,
		AcceptsIncomplete:   true,
	}
	resp, err := b.client.DeprovisionInstance(&req)
	if err != nil {
		return err
	}
	if resp != nil && resp.OperationKey != nil {
		instance.BrokerData.LastOperationKey = string(*resp.OperationKey)
		err = updateBrokerData(ctx, instance)
	}
	return err
}

func (b *brokerClient) BindApp(ctx context.Context, instance *ServiceInstance, app bind.App, params BindAppParameters, evt *event.Event, requestID string) (map[string]string, error) {
	if instance.BrokerData == nil {
		return nil, ErrInvalidBrokerData
	}
	id, err := idForEvent(evt)
	if err != nil {
		return nil, err
	}
	appGUID, err := app.GetUUID(ctx)
	if err != nil {
		return nil, err
	}
	bindID, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	bind := BrokerInstanceBind{
		UUID:       bindID.String(),
		Parameters: params,
	}
	req := osb.BindRequest{
		ServiceID:           instance.BrokerData.ServiceID,
		InstanceID:          instance.BrokerData.UUID,
		PlanID:              instance.BrokerData.PlanID,
		BindingID:           bind.UUID,
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
	for k, v := range b.broker.Config.Context {
		req.Context[k] = v
	}
	resp, err := b.client.Bind(&req)
	if osb.IsAsyncBindingOperationsNotAllowedError(err) {
		req.AcceptsIncomplete = false
		resp, err = b.client.Bind(&req)
	}
	if err != nil {
		return nil, err
	}
	if resp.OperationKey != nil {
		bind.OperationKey = string(*resp.OperationKey)
		instance.BrokerData.LastOperationKey = string(*resp.OperationKey)
	}
	envs := make(map[string]string)
	for k, v := range resp.Credentials {
		switch s := v.(type) {
		case string:
			envs[k] = s
		case int:
			envs[k] = strconv.Itoa(s)
		}
	}
	if instance.BrokerData.Binds == nil {
		instance.BrokerData.Binds = make(map[string]BrokerInstanceBind)
	}
	instance.BrokerData.Binds[app.GetName()] = bind
	return envs, updateBrokerData(ctx, instance)
}

func (b *brokerClient) UnbindApp(ctx context.Context, instance *ServiceInstance, app bind.App, evt *event.Event, requestID string) error {
	if instance.BrokerData == nil {
		return ErrInvalidBrokerData
	}
	id, err := idForEvent(evt)
	if err != nil {
		return err
	}
	req := osb.UnbindRequest{
		InstanceID:          instance.BrokerData.UUID,
		BindingID:           instance.BrokerData.Binds[app.GetName()].UUID,
		ServiceID:           instance.BrokerData.ServiceID,
		PlanID:              instance.BrokerData.PlanID,
		OriginatingIdentity: id,
		AcceptsIncomplete:   true,
	}
	resp, err := b.client.Unbind(&req)
	if osb.IsAsyncBindingOperationsNotAllowedError(err) {
		req.AcceptsIncomplete = false
		resp, err = b.client.Unbind(&req)
	}
	if err != nil {
		return err
	}
	delete(instance.BrokerData.Binds, app.GetName())
	if resp != nil && resp.OperationKey != nil {
		instance.BrokerData.LastOperationKey = string(*resp.OperationKey)
		err = updateBrokerData(ctx, instance)
	}
	return err
}

func (b *brokerClient) Status(ctx context.Context, instance *ServiceInstance, requestID string) (string, error) {
	if instance.BrokerData == nil {
		return "", ErrInvalidBrokerData
	}
	origID, err := json.Marshal(map[string]interface{}{
		"team": instance.TeamOwner,
	})
	if err != nil {
		return "", err
	}
	opKey := osb.OperationKey(instance.BrokerData.LastOperationKey)
	op, err := b.client.PollLastOperation(&osb.LastOperationRequest{
		ServiceID:  &instance.BrokerData.ServiceID,
		PlanID:     &instance.BrokerData.PlanID,
		InstanceID: instance.BrokerData.UUID,
		OriginatingIdentity: &osb.OriginatingIdentity{
			Platform: "tsuru",
			Value:    string(origID),
		},
		OperationKey: &opKey,
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

func (b *brokerClient) Info(ctx context.Context, instance *ServiceInstance, requestID string) ([]map[string]string, error) {
	var params []map[string]string
	for k, v := range instance.Parameters {
		params = append(params, map[string]string{
			"label": k,
			"value": fmt.Sprintf("%v", v),
		})
	}
	return params, nil
}

func (b *brokerClient) Plans(ctx context.Context, _, _ string) ([]Plan, error) {
	_, s, err := b.getService(ctx, b.service, b.broker.Name)
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
func (b *brokerClient) Proxy(ctx context.Context, opts *ProxyOpts) error {
	return fmt.Errorf("service proxy is not available for broker services")
}

// UnbindJob is a no-op for OSB API implementations
func (b *brokerClient) UnbindJob(ctx context.Context, instance *ServiceInstance, job *jobTypes.Job, evt *event.Event, requestID string) error {
	return nil
}

// BindJob is a no-op for OSB API implementations
func (b *brokerClient) BindJob(ctx context.Context, instance *ServiceInstance, job *jobTypes.Job, evt *event.Event, requestID string) (map[string]string, error) {
	return nil, nil
}

func (b *brokerClient) getCatalog(ctx context.Context, name string) (*osb.CatalogResponse, error) {
	catalog, err := servicemanager.ServiceBrokerCatalogCache.Load(ctx, name)
	if err != nil || catalog == nil {
		response, err := b.client.GetCatalog()
		if err != nil {
			return nil, err
		}
		cat := convertResponseToCatalog(*response)
		err = servicemanager.ServiceBrokerCatalogCache.Save(ctx, name, cat)
		if err != nil {
			log.Errorf("[Broker=%v] error caching catalog: %v.", name, err)
		}
		return response, nil
	}

	cat := convertCatalogToResponse(*catalog)
	return &cat, nil
}

func (b *brokerClient) getService(ctx context.Context, name, catalogName string) (Service, osb.Service, error) {
	cat, err := b.getCatalog(ctx, catalogName)
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

func updateBrokerData(ctx context.Context, instance *ServiceInstance) error {
	collection, err := storagev2.ServiceInstancesCollection()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		mongoBSON.M{"name": instance.Name, "service_name": instance.ServiceName},
		mongoBSON.M{"$set": mongoBSON.M{"broker_data": instance.BrokerData}},
	)

	return err
}
