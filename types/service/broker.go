// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"
)

var (
	ErrServiceBrokerAlreadyExists = errors.New("service broker already exists with the same name")
	ErrServiceBrokerNotFound      = errors.New("service broker not found")
)

// Broker contains the data required to request services to a
// Service Broker API.
type Broker struct {
	// Name is the name of the Service Broker.
	Name string
	// URL is the URL of the Service Broker API endpoint.
	URL string
	// AuthConfig is the credentials needed to interact with the API.
	AuthConfig *AuthConfig
}

// AuthConfig is a union-type representing the possible auth configurations a
// client may use to authenticate to a broker.  Currently, only basic auth is
// supported.
type AuthConfig struct {
	BasicAuthConfig *BasicAuthConfig
	BearerConfig    *BearerConfig
}

// BasicAuthConfig represents a set of basic auth credentials.
type BasicAuthConfig struct {
	// Username is the basic auth username.
	Username string
	// Password is the basic auth password.
	Password string
}

// BearerConfig represents bearer token credentials.
type BearerConfig struct {
	// Token is the bearer token.
	Token string
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
