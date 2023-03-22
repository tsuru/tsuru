// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"net/url"

	"github.com/tsuru/tsuru/types/app/image"
)

type App interface {
	GetName() string
	GetPool() string
	GetTeamOwner() string
	GetTeamsName() []string
	GetPlatform() string
	GetPlatformVersion() string
	GetRegistry() (image.ImageRegistry, error)
	GetDeploys() uint
	GetUpdatePlatform() bool
}

// EnvVar represents a environment variable for an app.
type EnvVar struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Alias     string `json:"alias"`
	Public    bool   `json:"public"`
	ManagedBy string `json:"managedBy,omitempty"`
}

type ServiceEnvVar struct {
	EnvVar       `bson:",inline"`
	ServiceName  string `json:"-"`
	InstanceName string `json:"-"`
}

type AppRouter struct {
	Name         string            `json:"name"`
	Opts         map[string]string `json:"opts"`
	Address      string            `json:"address" bson:"-"`
	Addresses    []string          `json:"addresses" bson:"-"`
	Type         string            `json:"type" bson:"-"`
	Status       string            `json:"status,omitempty" bson:"-"`
	StatusDetail string            `json:"status-detail,omitempty" bson:"-"`
}

type RoutableAddresses struct {
	Prefix    string
	Addresses []*url.URL
	ExtraData map[string]string
}

type Filter struct {
	Name        string
	NameMatches string
	Platform    string
	TeamOwner   string
	UserOwner   string
	Pool        string
	Pools       []string
	Statuses    []string
	Locked      bool
	Tags        []string
	Extra       map[string][]string
}

type AppService interface {
	GetByName(ctx context.Context, name string) (App, error)
	List(ctx context.Context, filter *Filter) ([]App, error)
}
