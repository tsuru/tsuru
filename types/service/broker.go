// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
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
	// CacheExpirationSeconds is a time duration in seconds that the Service
	// Broker catalog is kept in cache
	CacheExpirationSeconds int
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
	Insert(context.Context, Broker) error
	Update(context.Context, string, Broker) error
	Delete(context.Context, string) error
	FindAll(ctx context.Context) ([]Broker, error)
	Find(ctx context.Context, name string) (Broker, error)
}

type ServiceBrokerService interface {
	Create(context.Context, Broker) error
	Update(context.Context, string, Broker) error
	Delete(context.Context, string) error
	Find(context.Context, string) (Broker, error)
	List(context.Context) ([]Broker, error)
}
