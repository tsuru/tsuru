// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package service

import "context"

// BrokerCatalog contains the data required to request services to a
// Service Broker API.
// Most of the fields are copied from osb client definition.
type BrokerCatalog struct {
	Services []BrokerService
}

// BrokerService is an available service listed in a broker's catalog.
type BrokerService struct {
	// ID is a globally unique ID that identifies the service.
	ID string
	// Name is the service's display name.
	Name string
	// Description is a brief description of the service, suitable for
	// printing by a CLI.
	Description string
	// Plans is the list of the Plans for a service.  Plans represent
	// different tiers.
	Plans []BrokerPlan
}

// BrokerPlan is a plan (or tier) within a service offering.
type BrokerPlan struct {
	// ID is a globally unique ID that identifies the plan.
	ID string
	// Name is the plan's display name.
	Name string
	// Description is a brief description of the plan, suitable for
	// printing by a CLI.
	Description string
	Schemas     interface{}
}

type ServiceBrokerCatalogCacheService interface {
	Save(context.Context, string, BrokerCatalog) error
	Load(context.Context, string) (*BrokerCatalog, error)
}
