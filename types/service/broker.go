// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"

	osb "github.com/pmorie/go-open-service-broker-client/v2"
)

var (
	ErrServiceBrokerAlreadyExists = errors.New("service broker already exists with the same name")
	ErrServiceBrokerNotFound      = errors.New("service broker not found")
)

type Broker struct {
	Name       string
	URL        string
	AuthConfig *osb.AuthConfig
}

type ServiceBrokerStorage interface {
	Insert(Broker) error
	Update(string, Broker) error
	Delete(string) error
	FindAll() ([]Broker, error)
	Find(string) (Broker, error)
}

type ServiceBrokerService interface {
	Create(Broker) error
	Update(string, Broker) error
	Delete(string) error
	Find(string) (Broker, error)
	List() ([]Broker, error)
}
