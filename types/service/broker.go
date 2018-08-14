// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"
	"time"
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
	// Config is the configuration used to setup a client for the broker
	Config BrokerConfig
}

// BrokerConfig exposes configuration used to talk to the broker API.
// Most of the fields are copied from osb client definition.
type BrokerConfig struct {
	// AuthConfig is the auth configuration the client should use to authenticate
	// to the broker.
	AuthConfig *AuthConfig
	// Insecure represents whether the 'InsecureSkipVerify' TLS configuration
	// field should be set.  If the TLSConfig field is set and this field is
	// set to true, it overrides the value in the TLSConfig field.
	Insecure bool
	// Context is a set of key/value pairs that are going to be added to every
	// request to the Service Broker
	Context map[string]interface{}
	// CacheExpiration is a time period that the Service Broker catalog is kept
	// in cache
	CacheExpiration *time.Duration
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
