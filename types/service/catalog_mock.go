// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import "context"

var _ ServiceBrokerCatalogCacheService = &MockServiceBrokerCatalogCacheService{}

// MockServiceBrokerCatalogCacheService implements ServiceBrokerCatalogCacheService interface
type MockServiceBrokerCatalogCacheService struct {
	OnSave func(string, BrokerCatalog) error
	OnLoad func(string) (*BrokerCatalog, error)
}

func (m *MockServiceBrokerCatalogCacheService) Save(ctx context.Context, brokerName string, catalog BrokerCatalog) error {
	if m.OnSave == nil {
		return nil
	}
	return m.OnSave(brokerName, catalog)
}

func (m *MockServiceBrokerCatalogCacheService) Load(ctx context.Context, brokerName string) (*BrokerCatalog, error) {
	if m.OnLoad == nil {
		return nil, nil
	}
	return m.OnLoad(brokerName)
}
