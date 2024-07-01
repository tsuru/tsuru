// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import "context"

var _ ServiceBrokerService = &MockServiceBrokerService{}

// MockServiceBrokerService implements ServiceBrokerService interface
type MockServiceBrokerService struct {
	OnCreate func(Broker) error
	OnUpdate func(string, Broker) error
	OnDelete func(string) error
	OnFind   func(string) (Broker, error)
	OnList   func() ([]Broker, error)
}

func (m *MockServiceBrokerService) Create(broker Broker) error {
	if m.OnCreate == nil {
		return nil
	}
	return m.OnCreate(broker)
}

func (m *MockServiceBrokerService) Update(ctx context.Context, name string, broker Broker) error {
	if m.OnUpdate == nil {
		return nil
	}
	return m.OnUpdate(name, broker)
}

func (m *MockServiceBrokerService) Delete(name string) error {
	if m.OnDelete == nil {
		return nil
	}
	return m.OnDelete(name)
}

func (m *MockServiceBrokerService) Find(ctx context.Context, name string) (Broker, error) {
	if m.OnFind == nil {
		return Broker{}, nil
	}
	return m.OnFind(name)
}

func (m *MockServiceBrokerService) List(ctx context.Context) ([]Broker, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList()
}
